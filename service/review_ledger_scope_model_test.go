package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
)

func TestModel_ReviewLedgerScopedByTask(t *testing.T) {
	type reviewAction struct {
		task       string
		reviewer   catalog.PersonID
		conclusion arbiter.ReviewConclusion
		wantFault  string
	}
	type reviewFact struct {
		reviewer   catalog.PersonID
		conclusion arbiter.ReviewConclusion
	}
	type reportExpectation struct {
		task    string
		reviews []reviewFact
	}
	type finalExpectation struct {
		task       string
		wantFault  string
		wantStatus inspection.Status
		wantFinal  inspection.FinalType
	}

	ctx := context.Background()
	cases := []struct {
		name    string
		reviews []reviewAction
		reports []reportExpectation
		final   finalExpectation
	}{
		{
			name: "foreign pass reviews do not authorize current task",
			reviews: []reviewAction{
				{task: "alpha", reviewer: catalog.FixedReviewerA, conclusion: arbiter.ReviewPass},
				{task: "alpha", reviewer: catalog.FixedReviewerB, conclusion: arbiter.ReviewPass},
			},
			reports: []reportExpectation{
				{task: "alpha", reviews: []reviewFact{
					{reviewer: catalog.FixedReviewerA, conclusion: arbiter.ReviewPass},
					{reviewer: catalog.FixedReviewerB, conclusion: arbiter.ReviewPass},
				}},
				{task: "beta"},
			},
			final: finalExpectation{
				task: "beta", wantFault: CodeFinalizeConflict, wantStatus: inspection.StatusPendingReview,
			},
		},
		{
			name: "foreign fail reviews do not pollute current admissible task",
			reviews: []reviewAction{
				{task: "alpha", reviewer: catalog.FixedReviewerA, conclusion: arbiter.ReviewFail},
				{task: "alpha", reviewer: catalog.FixedReviewerB, conclusion: arbiter.ReviewFail},
				{task: "beta", reviewer: catalog.FixedReviewerA, conclusion: arbiter.ReviewPass},
				{task: "beta", reviewer: catalog.FixedReviewerB, conclusion: arbiter.ReviewPass},
			},
			reports: []reportExpectation{
				{task: "beta", reviews: []reviewFact{
					{reviewer: catalog.FixedReviewerA, conclusion: arbiter.ReviewPass},
					{reviewer: catalog.FixedReviewerB, conclusion: arbiter.ReviewPass},
				}},
			},
			final: finalExpectation{
				task: "beta", wantStatus: inspection.StatusAdmissible, wantFinal: inspection.FinalAdmissible,
			},
		},
		{
			name: "duplicate review is rejected only after current task already has it",
			reviews: []reviewAction{
				{task: "alpha", reviewer: catalog.FixedReviewerA, conclusion: arbiter.ReviewPass},
				{task: "beta", reviewer: catalog.FixedReviewerA, conclusion: arbiter.ReviewPass},
				{task: "beta", reviewer: catalog.FixedReviewerA, conclusion: arbiter.ReviewPass, wantFault: CodeDuplicateReview},
			},
			reports: []reportExpectation{
				{task: "beta", reviews: []reviewFact{
					{reviewer: catalog.FixedReviewerA, conclusion: arbiter.ReviewPass},
				}},
			},
			final: finalExpectation{
				task: "beta", wantFault: CodeFinalizeConflict, wantStatus: inspection.StatusPendingReview,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, st := newFixtureService(t)
			defer st.Close()

			createReadyTask := func(t *testing.T, label string) inspection.TaskID {
				t.Helper()
				taskID := inspection.TaskID("task-" + label)
				batch := inspection.TankBatch("BATCH-SCOPE-" + label)
				recorder := "recorder-" + label
				blindCodes := []blindcode.BlindCode{
					blindcode.BlindCode("BCODE-" + label + "-A"),
					blindcode.BlindCode("BCODE-" + label + "-B"),
				}

				_, fault := svc.CreateTask(ctx, CreateTaskRequest{
					TaskID: taskID, FarmID: catalog.FixedFarmID, TankBatch: batch,
					Compartments:  []catalog.CompartmentCode{"A", "B"},
					Seals:         []catalog.SealCode{"seal-0001", "seal-0002"},
					RecorderModel: catalog.RecorderModel(recorder),
					RuleVersion:   catalog.FixedRuleVersion,
					Reviewers:     []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
				})
				if fault != nil {
					t.Fatalf("create %s: %v", label, fault)
				}

				for i, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
					_, fault := svc.SamplingConfirm(ctx, taskID, SamplingConfirmationRequest{
						OperationID:  inspection.OperationID("op-" + label + "-sample-" + strconvItoa(i)),
						Person:       sampler,
						FarmID:       catalog.FixedFarmID,
						TankBatch:    batch,
						Compartments: []catalog.CompartmentCode{"A", "B"},
						Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
						Generation:   1,
					})
					if fault != nil {
						t.Fatalf("sampling %s/%d: %v", label, i, fault)
					}
				}

				_, fault = svc.BlindSplit(ctx, taskID, BlindSplitRequest{
					OperationID: inspection.OperationID("op-" + label + "-split"),
					Generation:  1,
					Codes:       blindCodes,
				})
				if fault != nil {
					t.Fatalf("blind split %s: %v", label, fault)
				}

				_, fault = svc.AcquireOccupancy(ctx, taskID, OccupancyRequest{
					OperationID: inspection.OperationID("op-" + label + "-occupancy"),
					Generation:  1,
					Occupancies: []occupancy.Occupancy{
						{TaskID: string(taskID), ResourceType: occupancy.ResourcePlateWell, PlateID: "plate-" + label, Well: "A1", StartAt: 0, EndAt: 3600},
						{TaskID: string(taskID), ResourceType: occupancy.ResourceIncubator, IncubatorID: "inc-" + label, StartAt: 0, EndAt: 3600},
					},
				})
				if fault != nil {
					t.Fatalf("occupancy %s: %v", label, fault)
				}

				rules, _ := svc.Catalog().Rules(catalog.FixedRuleVersion)
				var cells []evidence.TemperatureCell
				n := int(rules.Temperature.WindowSeconds/rules.Temperature.SampleEverySeconds) + 1
				for i := 0; i < n; i++ {
					cells = append(cells, evidence.TemperatureCell{
						AtSeconds: int64(i * 60),
						Celsius:   evidence.FixedPoint{Value: 40, Scale: 1},
					})
				}
				_, fault = svc.ColdChainReadings(ctx, taskID, ColdChainReadingsRequest{
					OperationID: inspection.OperationID("op-" + label + "-cold"),
					Generation:  1,
					BaseTime:    0,
					RecorderID:  recorder,
					Cells:       cells,
				})
				if fault != nil {
					t.Fatalf("cold chain %s: %v", label, fault)
				}

				for i, code := range blindCodes {
					mustRead(t, svc, taskID, "op-"+label+"-anti-"+strconvItoa(i), evidence.EvidenceAntibiotic, string(code), "20.0")
				}
				for i, code := range blindCodes {
					mustRead(t, svc, taskID, "op-"+label+"-som-"+strconvItoa(i), evidence.EvidenceSomaticCell, string(code), "350")
					mustRead(t, svc, taskID, "op-"+label+"-col-"+strconvItoa(i), evidence.EvidenceColony, string(code), "50000")
				}
				for i, code := range blindCodes {
					mustRead(t, svc, taskID, "op-"+label+"-fp-"+strconvItoa(i), evidence.EvidenceFreezingPoint, string(code), "-53.0")
					mustRead(t, svc, taskID, "op-"+label+"-fat-"+strconvItoa(i), evidence.EvidenceFat, string(code), "3.5")
					mustRead(t, svc, taskID, "op-"+label+"-prot-"+strconvItoa(i), evidence.EvidenceProtein, string(code), "3.1")
				}

				snap, fault := svc.GetSnapshot(ctx, taskID)
				if fault != nil {
					t.Fatalf("snapshot %s: %v", label, fault)
				}
				if snap.Task.Status != inspection.StatusPendingReview {
					t.Fatalf("%s status = %s, want pending_review", label, snap.Task.Status)
				}
				return taskID
			}

			tasks := map[string]inspection.TaskID{
				"alpha": createReadyTask(t, "alpha"),
				"beta":  createReadyTask(t, "beta"),
			}

			reviewAttempts := map[string]int{}
			for _, action := range tc.reviews {
				reviewAttempts[action.task]++
				_, fault := svc.Review(ctx, tasks[action.task], ReviewRequest{
					OperationID: inspection.OperationID("op-" + tc.name + "-" + action.task + "-review-" + strconvItoa(reviewAttempts[action.task])),
					Generation:  1,
					Reviewer:    action.reviewer,
					Conclusion:  action.conclusion,
				})
				if action.wantFault == "" {
					if fault != nil {
						t.Fatalf("review %s/%s: %v", action.task, action.reviewer, fault)
					}
					continue
				}
				if fault == nil || fault.Code != action.wantFault {
					t.Fatalf("review %s/%s fault = %v, want %s", action.task, action.reviewer, fault, action.wantFault)
				}
			}

			for _, want := range tc.reports {
				report, fault := svc.BuildReport(ctx, tasks[want.task])
				if fault != nil {
					t.Fatalf("report %s: %v", want.task, fault)
				}
				if len(report.Reviews) != len(want.reviews) {
					t.Fatalf("report %s reviews = %+v, want %+v", want.task, report.Reviews, want.reviews)
				}
				byReviewer := map[catalog.PersonID]arbiter.ReviewConclusion{}
				for _, got := range report.Reviews {
					byReviewer[got.Reviewer] = got.Conclusion
				}
				for _, expected := range want.reviews {
					if byReviewer[expected.reviewer] != expected.conclusion {
						t.Fatalf("report %s reviewer %s = %s, want %s", want.task, expected.reviewer, byReviewer[expected.reviewer], expected.conclusion)
					}
				}
			}

			result, fault := svc.Finalize(ctx, tasks[tc.final.task], FinalizeRequest{
				OperationID: inspection.OperationID("op-" + tc.name + "-final"),
				Generation:  1,
				Outcome:     inspection.FinalAdmissible,
			})
			if tc.final.wantFault == "" {
				if fault != nil {
					t.Fatalf("finalize %s: %v", tc.final.task, fault)
				}
				if result.FinalType != tc.final.wantFinal || result.Credential == "" {
					t.Fatalf("finalize %s result = %+v, want %s with credential", tc.final.task, result, tc.final.wantFinal)
				}
			} else if fault == nil || fault.Code != tc.final.wantFault {
				t.Fatalf("finalize %s fault = %v, want %s", tc.final.task, fault, tc.final.wantFault)
			}

			snap, snapFault := svc.GetSnapshot(ctx, tasks[tc.final.task])
			if snapFault != nil {
				t.Fatalf("snapshot final task: %v", snapFault)
			}
			if snap.Task.Status != tc.final.wantStatus {
				t.Fatalf("final status = %s, want %s", snap.Task.Status, tc.final.wantStatus)
			}
		})
	}
}
