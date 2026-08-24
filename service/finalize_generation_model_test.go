package service

import (
	"context"
	"strconv"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_FinalizeGenerationGuardRejectsStaleTerminalRequests(t *testing.T) {
	ctx := context.Background()

	idempotencyExists := func(t *testing.T, svc *Service, taskID inspection.TaskID, opID inspection.OperationID) bool {
		t.Helper()
		var exists bool
		err := svc.Store().WithTx(ctx, func(tx store.Tx) error {
			_, ok, err := tx.GetIdempotency(ctx, taskID, opID)
			exists = ok
			return err
		})
		if err != nil {
			t.Fatalf("read idempotency: %v", err)
		}
		return exists
	}

	cases := []struct {
		name       string
		outcome    inspection.FinalType
		wantStatus inspection.Status
	}{
		{name: "stale_admissible", outcome: inspection.FinalAdmissible, wantStatus: inspection.StatusAdmissible},
		{name: "stale_quarantined", outcome: inspection.FinalQuarantined, wantStatus: inspection.StatusQuarantined},
		{name: "stale_cancelled", outcome: inspection.FinalCancelled, wantStatus: inspection.StatusCancelled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMemoryStore(catalog.NewFixedCatalog())
			svc := NewService(st, NewManualClock(1000))
			defer st.Close()

			taskID := inspection.TaskID("task-model-" + tc.name)
			op := func(suffix string) inspection.OperationID {
				return inspection.OperationID("op-model-" + tc.name + "-" + suffix)
			}

			if _, fault := svc.CreateTask(ctx, CreateTaskRequest{
				TaskID:        taskID,
				FarmID:        catalog.FixedFarmID,
				TankBatch:     inspection.TankBatch("batch-model-" + tc.name),
				Compartments:  []catalog.CompartmentCode{"A", "B"},
				Seals:         []catalog.SealCode{"seal-0001", "seal-0002"},
				RecorderModel: "recorder-x1",
				RuleVersion:   catalog.FixedRuleVersion,
				Reviewers:     []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
			}); fault != nil {
				t.Fatalf("create task: %v", fault)
			}

			for i, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
				if _, fault := svc.SamplingConfirm(ctx, taskID, SamplingConfirmationRequest{
					OperationID:  op("sample-" + strconv.Itoa(i)),
					Person:       sampler,
					FarmID:       catalog.FixedFarmID,
					TankBatch:    inspection.TankBatch("batch-model-" + tc.name),
					Compartments: []catalog.CompartmentCode{"A", "B"},
					Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
					Generation:   1,
				}); fault != nil {
					t.Fatalf("sampling confirm %d: %v", i, fault)
				}
			}

			if _, fault := svc.BlindSplit(ctx, taskID, BlindSplitRequest{
				OperationID: op("split"),
				Generation:  1,
				Codes:       []blindcode.BlindCode{"BCODE-A", "BCODE-B"},
			}); fault != nil {
				t.Fatalf("blind split: %v", fault)
			}

			if _, fault := svc.AcquireOccupancy(ctx, taskID, OccupancyRequest{
				OperationID: op("occupancy"),
				Generation:  1,
				Occupancies: []occupancy.Occupancy{
					{TaskID: string(taskID), ResourceType: occupancy.ResourcePlateWell, PlateID: "plate-model", Well: "A1", StartAt: 0, EndAt: 3600},
					{TaskID: string(taskID), ResourceType: occupancy.ResourceIncubator, IncubatorID: "inc-model", StartAt: 0, EndAt: 3600},
				},
			}); fault != nil {
				t.Fatalf("occupancy: %v", fault)
			}

			rules, ok := svc.Catalog().Rules(catalog.FixedRuleVersion)
			if !ok {
				t.Fatalf("fixed rules missing")
			}
			cells := make([]evidence.TemperatureCell, 0, int(rules.Temperature.WindowSeconds/rules.Temperature.SampleEverySeconds)+1)
			for i := 0; i <= int(rules.Temperature.WindowSeconds/rules.Temperature.SampleEverySeconds); i++ {
				cells = append(cells, evidence.TemperatureCell{
					AtSeconds: int64(i * 60),
					Celsius:   evidence.FixedPoint{Value: 40, Scale: 1},
				})
			}
			if _, fault := svc.ColdChainReadings(ctx, taskID, ColdChainReadingsRequest{
				OperationID: op("cold-chain"),
				Generation:  1,
				BaseTime:    0,
				RecorderID:  "recorder-x1",
				Cells:       cells,
			}); fault != nil {
				t.Fatalf("cold chain: %v", fault)
			}

			for i, code := range []string{"BCODE-A", "BCODE-B"} {
				if _, fault := svc.SubmitReading(ctx, taskID, ReadingRequest{
					OperationID: op("antibiotic-" + strconv.Itoa(i)),
					Generation:  1,
					Type:        evidence.EvidenceAntibiotic,
					BlindCode:   code,
					Well:        "A" + strconv.Itoa(i+1),
					Value:       "20.0",
				}); fault != nil {
					t.Fatalf("antibiotic reading %d: %v", i, fault)
				}
			}
			for i, code := range []string{"BCODE-A", "BCODE-B"} {
				if _, fault := svc.SubmitReading(ctx, taskID, ReadingRequest{
					OperationID: op("somatic-" + strconv.Itoa(i)),
					Generation:  1,
					Type:        evidence.EvidenceSomaticCell,
					BlindCode:   code,
					Value:       "350",
				}); fault != nil {
					t.Fatalf("somatic reading %d: %v", i, fault)
				}
				if _, fault := svc.SubmitReading(ctx, taskID, ReadingRequest{
					OperationID: op("colony-" + strconv.Itoa(i)),
					Generation:  1,
					Type:        evidence.EvidenceColony,
					BlindCode:   code,
					Value:       "50000",
				}); fault != nil {
					t.Fatalf("colony reading %d: %v", i, fault)
				}
			}
			for i, code := range []string{"BCODE-A", "BCODE-B"} {
				if _, fault := svc.SubmitReading(ctx, taskID, ReadingRequest{
					OperationID: op("freezing-point-" + strconv.Itoa(i)),
					Generation:  1,
					Type:        evidence.EvidenceFreezingPoint,
					BlindCode:   code,
					Value:       "-53.0",
				}); fault != nil {
					t.Fatalf("freezing point reading %d: %v", i, fault)
				}
				if _, fault := svc.SubmitReading(ctx, taskID, ReadingRequest{
					OperationID: op("fat-" + strconv.Itoa(i)),
					Generation:  1,
					Type:        evidence.EvidenceFat,
					BlindCode:   code,
					Value:       "3.5",
				}); fault != nil {
					t.Fatalf("fat reading %d: %v", i, fault)
				}
				if _, fault := svc.SubmitReading(ctx, taskID, ReadingRequest{
					OperationID: op("protein-" + strconv.Itoa(i)),
					Generation:  1,
					Type:        evidence.EvidenceProtein,
					BlindCode:   code,
					Value:       "3.1",
				}); fault != nil {
					t.Fatalf("protein reading %d: %v", i, fault)
				}
			}

			for _, reviewer := range []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB} {
				if _, fault := svc.Review(ctx, taskID, ReviewRequest{
					OperationID: op("review-" + string(reviewer)),
					Generation:  1,
					Reviewer:    reviewer,
					Conclusion:  arbiter.ReviewPass,
				}); fault != nil {
					t.Fatalf("review %s: %v", reviewer, fault)
				}
			}

			rejudge, fault := svc.Rejudge(ctx, taskID, RejudgementRequest{
				OperationID:  op("rejudge"),
				Generation:   1,
				Reason:       arbiter.ReasonSuspectPositive,
				BlindCodes:   []string{"BCODE-A"},
				Compartments: []catalog.CompartmentCode{"A"},
				Wells:        []string{"A1"},
			})
			if fault != nil {
				t.Fatalf("rejudge: %v", fault)
			}
			if rejudge.Generation != 2 {
				t.Fatalf("rejudge generation = %d, want 2", rejudge.Generation)
			}

			before, fault := svc.GetSnapshot(ctx, taskID)
			if fault != nil {
				t.Fatalf("snapshot before stale finalize: %v", fault)
			}
			staleOp := op("final-" + string(tc.outcome))
			result, fault := svc.Finalize(ctx, taskID, FinalizeRequest{
				OperationID: staleOp,
				Generation:  1,
				Outcome:     tc.outcome,
			})
			if fault == nil || fault.Code != CodeStaleGeneration {
				t.Fatalf("stale finalize fault = %v, result = %+v; want stale_generation and no result", fault, result)
			}
			if result != nil {
				t.Fatalf("stale finalize returned result: %+v", result)
			}

			after, fault := svc.GetSnapshot(ctx, taskID)
			if fault != nil {
				t.Fatalf("snapshot after stale finalize: %v", fault)
			}
			if after.Task.Generation != 2 {
				t.Fatalf("generation after stale finalize = %d, want 2", after.Task.Generation)
			}
			if after.Task.Status != inspection.StatusPendingReview {
				t.Fatalf("status after stale finalize = %s, want pending_review", after.Task.Status)
			}
			if after.Task.FinalType != "" {
				t.Fatalf("final type after stale finalize = %s, want empty", after.Task.FinalType)
			}
			if after.FinalDecision != nil {
				t.Fatalf("final decision after stale finalize = %+v, want nil", after.FinalDecision)
			}
			if len(after.Audit) != len(before.Audit) {
				t.Fatalf("audit events after stale finalize = %d, want unchanged %d", len(after.Audit), len(before.Audit))
			}
			for _, ev := range after.Audit {
				if ev.EventType == inspection.EventFinalized {
					t.Fatalf("stale finalize wrote finalized audit event: %+v", ev)
				}
			}
			if idempotencyExists(t, svc, taskID, staleOp) {
				t.Fatalf("stale finalize wrote idempotency record for %s", staleOp)
			}

			current, fault := svc.Finalize(ctx, taskID, FinalizeRequest{
				OperationID: staleOp,
				Generation:  2,
				Outcome:     tc.outcome,
			})
			if fault != nil {
				t.Fatalf("current generation finalize: %v", fault)
			}
			if current == nil || current.Generation != 2 || current.FinalType != tc.outcome || current.Credential == "" {
				t.Fatalf("current finalize result = %+v, want generation 2 %s with credential", current, tc.outcome)
			}

			final, fault := svc.GetSnapshot(ctx, taskID)
			if fault != nil {
				t.Fatalf("snapshot after current finalize: %v", fault)
			}
			if final.Task.Status != tc.wantStatus || final.Task.FinalType != tc.outcome {
				t.Fatalf("final task = status %s type %s, want %s/%s", final.Task.Status, final.Task.FinalType, tc.wantStatus, tc.outcome)
			}
			if final.FinalDecision == nil || final.FinalDecision.FinalType != tc.outcome || final.FinalDecision.Credential == "" {
				t.Fatalf("final decision = %+v, want %s with credential", final.FinalDecision, tc.outcome)
			}
			if !idempotencyExists(t, svc, taskID, staleOp) {
				t.Fatalf("current finalize did not write idempotency record for %s", staleOp)
			}
			if len(final.Audit) != len(after.Audit)+1 {
				t.Fatalf("audit events after current finalize = %d, want %d", len(final.Audit), len(after.Audit)+1)
			}
			ev := final.Audit[len(final.Audit)-1]
			if ev.EventType != inspection.EventFinalized || ev.Generation != 2 || ev.Detail != string(tc.outcome) {
				t.Fatalf("final audit event = %+v, want finalized generation 2 detail %s", ev, tc.outcome)
			}
		})
	}
}
