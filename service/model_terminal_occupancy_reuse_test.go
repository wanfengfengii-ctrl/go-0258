package service

import (
	"context"
	"sync"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
)

func TestModel_TerminalOccupancyReuse(t *testing.T) {
	ctx := context.Background()
	sharedLeases := []occupancy.Occupancy{
		{ResourceType: occupancy.ResourcePlateWell, PlateID: "plate-terminal-reuse", Well: "B3", StartAt: 1000, EndAt: 4600},
		{ResourceType: occupancy.ResourceIncubator, IncubatorID: "inc-terminal-reuse", StartAt: 1000, EndAt: 4600},
	}

	createTask := func(t *testing.T, svc *Service, id inspection.TaskID, batch inspection.TankBatch) {
		t.Helper()
		_, fault := svc.CreateTask(ctx, CreateTaskRequest{
			TaskID:        id,
			FarmID:        catalog.FixedFarmID,
			TankBatch:     batch,
			Compartments:  []catalog.CompartmentCode{"A", "B"},
			Seals:         []catalog.SealCode{"seal-0001", "seal-0002"},
			RecorderModel: "recorder-x1",
			RuleVersion:   catalog.FixedRuleVersion,
			Reviewers:     []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
		})
		if fault != nil {
			t.Fatalf("create task %s: %v", id, fault)
		}
	}

	confirmSamplingFor := func(t *testing.T, svc *Service, id inspection.TaskID, batch inspection.TankBatch, opPrefix string) {
		t.Helper()
		for i, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
			_, fault := svc.SamplingConfirm(ctx, id, SamplingConfirmationRequest{
				OperationID:  inspection.OperationID(opPrefix + "-sample-" + strconvItoa(i)),
				Person:       sampler,
				FarmID:       catalog.FixedFarmID,
				TankBatch:    batch,
				Compartments: []catalog.CompartmentCode{"A", "B"},
				Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
				Generation:   1,
			})
			if fault != nil {
				t.Fatalf("sampling confirm %s/%d: %v", id, i, fault)
			}
		}
	}

	splitBlindFor := func(t *testing.T, svc *Service, id inspection.TaskID, codes []blindcode.BlindCode, opPrefix string) {
		t.Helper()
		_, fault := svc.BlindSplit(ctx, id, BlindSplitRequest{
			OperationID: inspection.OperationID(opPrefix + "-split"),
			Generation:  1,
			Codes:       codes,
		})
		if fault != nil {
			t.Fatalf("blind split %s: %v", id, fault)
		}
	}

	prepareForOccupancy := func(t *testing.T, svc *Service, id inspection.TaskID, batch inspection.TankBatch, codes []blindcode.BlindCode, opPrefix string) {
		t.Helper()
		createTask(t, svc, id, batch)
		confirmSamplingFor(t, svc, id, batch, opPrefix)
		splitBlindFor(t, svc, id, codes, opPrefix)
	}

	acquireShared := func(svc *Service, id inspection.TaskID, op string) (*OccupancyResult, *Fault) {
		return svc.AcquireOccupancy(ctx, id, OccupancyRequest{
			OperationID: inspection.OperationID(op),
			Generation:  1,
			Occupancies: sharedLeases,
		})
	}

	completeInspection := func(t *testing.T, svc *Service, id inspection.TaskID, codes []blindcode.BlindCode, opPrefix string) {
		t.Helper()
		writeColdChain(t, svc, id)
		for i, code := range codes {
			mustRead(t, svc, id, opPrefix+"-anti-"+strconvItoa(i), evidence.EvidenceAntibiotic, string(code), "20.0")
		}
		for i, code := range codes {
			mustRead(t, svc, id, opPrefix+"-som-"+strconvItoa(i), evidence.EvidenceSomaticCell, string(code), "350")
			mustRead(t, svc, id, opPrefix+"-col-"+strconvItoa(i), evidence.EvidenceColony, string(code), "50000")
		}
		for i, code := range codes {
			mustRead(t, svc, id, opPrefix+"-fp-"+strconvItoa(i), evidence.EvidenceFreezingPoint, string(code), "-53.0")
			mustRead(t, svc, id, opPrefix+"-fat-"+strconvItoa(i), evidence.EvidenceFat, string(code), "3.5")
			mustRead(t, svc, id, opPrefix+"-prot-"+strconvItoa(i), evidence.EvidenceProtein, string(code), "3.1")
		}
	}

	assertTerminalBarrier := func(t *testing.T, svc *Service, id inspection.TaskID, opPrefix string) {
		t.Helper()
		_, fault := svc.SubmitReading(ctx, id, ReadingRequest{
			OperationID: inspection.OperationID(opPrefix + "-late-reading"),
			Generation:  1,
			Type:        evidence.EvidenceAntibiotic,
			BlindCode:   "unused-late-code",
			Value:       "20.0",
		})
		if fault == nil || fault.Code != CodeTerminalState {
			t.Fatalf("late reading fault = %v, want terminal_state", fault)
		}
		_, fault = svc.Review(ctx, id, ReviewRequest{
			OperationID: inspection.OperationID(opPrefix + "-late-review"),
			Generation:  1,
			Reviewer:    catalog.FixedReviewerA,
			Conclusion:  "pass",
		})
		if fault == nil || fault.Code != CodeTerminalState {
			t.Fatalf("late review fault = %v, want terminal_state", fault)
		}
	}

	cases := []struct {
		name              string
		finalOutcome      inspection.FinalType
		concurrentFinal   bool
		instrumentFailure bool
		wantReuse         bool
		wantFaultCode     string
	}{
		{name: "completed_cancelled_terminal", finalOutcome: inspection.FinalCancelled, wantReuse: true},
		{name: "admissible_terminal", finalOutcome: inspection.FinalAdmissible, wantReuse: true},
		{name: "concurrent_terminal_winner", concurrentFinal: true, wantReuse: true},
		{name: "instrument_failure_keeps_lease_active", instrumentFailure: true, wantReuse: false, wantFaultCode: CodeOccupancyConflict},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, st := newFixtureService(t)
			defer st.Close()

			firstID := inspection.TaskID(tc.name + "-first")
			firstBatch := inspection.TankBatch("BATCH-" + tc.name + "-001")
			firstCodes := []blindcode.BlindCode{
				blindcode.BlindCode("BLIND-" + tc.name + "-A"),
				blindcode.BlindCode("BLIND-" + tc.name + "-B"),
			}
			prepareForOccupancy(t, svc, firstID, firstBatch, firstCodes, tc.name+"-first")
			if _, fault := acquireShared(svc, firstID, tc.name+"-first-occ"); fault != nil {
				t.Fatalf("first occupancy: %v", fault)
			}

			if tc.instrumentFailure {
				writeColdChain(t, svc, firstID)
				_, fault := svc.SubmitReading(ctx, firstID, ReadingRequest{
					OperationID:    inspection.OperationID(tc.name + "-instrument-timeout"),
					Generation:     1,
					Type:           evidence.EvidenceAntibiotic,
					BlindCode:      string(firstCodes[0]),
					Well:           "B3",
					InstrumentType: "plate-reader",
					ScriptResult:   "timeout",
					ErrorClass:     ErrClassTimeout,
				})
				if fault != nil {
					t.Fatalf("instrument failure should be recorded without releasing resources: %v", fault)
				}
			} else {
				completeInspection(t, svc, firstID, firstCodes, tc.name+"-first")
				passReviews(t, svc, firstID)
				if tc.concurrentFinal {
					outcomes := []inspection.FinalType{
						inspection.FinalAdmissible,
						inspection.FinalQuarantined,
						inspection.FinalCancelled,
					}
					start := make(chan struct{})
					var wg sync.WaitGroup
					faults := make([]*Fault, len(outcomes))
					for i, outcome := range outcomes {
						wg.Add(1)
						go func(i int, outcome inspection.FinalType) {
							defer wg.Done()
							<-start
							_, faults[i] = svc.Finalize(ctx, firstID, FinalizeRequest{
								OperationID: inspection.OperationID(tc.name + "-final-" + string(outcome)),
								Generation:  1,
								Outcome:     outcome,
							})
						}(i, outcome)
					}
					close(start)
					wg.Wait()

					winners := 0
					for _, fault := range faults {
						if fault == nil {
							winners++
						}
					}
					if winners != 1 {
						t.Fatalf("concurrent final winners = %d, want 1 (faults=%v)", winners, faults)
					}
				} else if _, fault := svc.Finalize(ctx, firstID, FinalizeRequest{
					OperationID: inspection.OperationID(tc.name + "-final"),
					Generation:  1,
					Outcome:     tc.finalOutcome,
				}); fault != nil {
					t.Fatalf("finalize %s: %v", tc.finalOutcome, fault)
				}

				snap, fault := svc.GetSnapshot(ctx, firstID)
				if fault != nil {
					t.Fatalf("snapshot after final: %v", fault)
				}
				if !snap.Task.Status.IsTerminal() {
					t.Fatalf("status = %s, want terminal", snap.Task.Status)
				}
				assertTerminalBarrier(t, svc, firstID, tc.name+"-first")
			}

			secondID := inspection.TaskID(tc.name + "-second")
			secondBatch := inspection.TankBatch("BATCH-" + tc.name + "-002")
			secondCodes := []blindcode.BlindCode{
				blindcode.BlindCode("BLIND-" + tc.name + "-C"),
				blindcode.BlindCode("BLIND-" + tc.name + "-D"),
			}
			prepareForOccupancy(t, svc, secondID, secondBatch, secondCodes, tc.name+"-second")
			got, fault := acquireShared(svc, secondID, tc.name+"-second-occ")
			if tc.wantReuse {
				if fault != nil {
					t.Fatalf("second task should reuse finalized resources, got fault: %v", fault)
				}
				if len(got.Acquired) != len(sharedLeases) {
					t.Fatalf("second task acquired %d resources, want %d", len(got.Acquired), len(sharedLeases))
				}
			} else if fault == nil || fault.Code != tc.wantFaultCode {
				t.Fatalf("second task fault = %v, want %s", fault, tc.wantFaultCode)
			}
		})
	}
}
