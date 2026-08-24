package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

func TestModel_RejudgementReasonDoesNotCrossGeneration(t *testing.T) {
	ctx := context.Background()
	svc, st := newFixtureService(t)
	defer st.Close()

	id := createFixtureTask(t, svc)
	advanceToReview(t, svc, id)

	request := func(op inspection.OperationID, generation inspection.Generation) RejudgementRequest {
		return RejudgementRequest{
			OperationID:  op,
			Generation:   generation,
			Reason:       arbiter.ReasonSuspectPositive,
			BlindCodes:   []string{"BCODE-A"},
			Compartments: []catalog.CompartmentCode{"A"},
			Wells:        []string{"A1"},
		}
	}

	cases := []struct {
		name                 string
		req                  RejudgementRequest
		wantFault            string
		wantResultGeneration inspection.Generation
		wantTaskGeneration   inspection.Generation
		wantRejudgements     []int64
		wantRejudgedAudit    []inspection.Generation
	}{
		{
			name:                 "first suspect positive rejudgement advances generation one",
			req:                  request("op-rejudge-generation-1", 1),
			wantResultGeneration: 2,
			wantTaskGeneration:   2,
			wantRejudgements:     []int64{1},
			wantRejudgedAudit:    []inspection.Generation{1},
		},
		{
			name:               "old generation request is stale and writes no second evidence",
			req:                request("op-rejudge-generation-1-stale", 1),
			wantFault:          CodeStaleGeneration,
			wantTaskGeneration: 2,
			wantRejudgements:   []int64{1},
			wantRejudgedAudit:  []inspection.Generation{1},
		},
		{
			name:               "reused operation id in new generation conflicts without side effects",
			req:                request("op-rejudge-generation-1", 2),
			wantFault:          CodeContentConflict,
			wantTaskGeneration: 2,
			wantRejudgements:   []int64{1},
			wantRejudgedAudit:  []inspection.Generation{1},
		},
		{
			name:                 "same suspect positive reason is accepted in next generation",
			req:                  request("op-rejudge-generation-2", 2),
			wantResultGeneration: 3,
			wantTaskGeneration:   3,
			wantRejudgements:     []int64{1, 2},
			wantRejudgedAudit:    []inspection.Generation{1, 2},
		},
		{
			name:               "accepted next generation request becomes stale on replay by generation",
			req:                request("op-rejudge-generation-2-stale", 2),
			wantFault:          CodeStaleGeneration,
			wantTaskGeneration: 3,
			wantRejudgements:   []int64{1, 2},
			wantRejudgedAudit:  []inspection.Generation{1, 2},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, fault := svc.Rejudge(ctx, id, tc.req)
			if tc.wantFault == "" {
				if fault != nil {
					t.Fatalf("Rejudge fault = %v, want nil", fault)
				}
				if res == nil {
					t.Fatal("Rejudge result is nil")
				}
				if res.Generation != tc.wantResultGeneration {
					t.Fatalf("result generation = %d, want %d", res.Generation, tc.wantResultGeneration)
				}
			} else {
				if fault == nil {
					t.Fatalf("Rejudge fault is nil, want %s", tc.wantFault)
				}
				if fault.Code != tc.wantFault {
					t.Fatalf("fault code = %s, want %s", fault.Code, tc.wantFault)
				}
				if res != nil {
					t.Fatalf("Rejudge result = %+v, want nil", res)
				}
			}

			snap, snapFault := svc.GetSnapshot(ctx, id)
			if snapFault != nil {
				t.Fatalf("snapshot fault = %v", snapFault)
			}
			if snap.Task.Generation != tc.wantTaskGeneration {
				t.Fatalf("snapshot task generation = %d, want %d", snap.Task.Generation, tc.wantTaskGeneration)
			}
			if len(snap.Rejudgements) != len(tc.wantRejudgements) {
				t.Fatalf("snapshot rejudgements = %d, want %d", len(snap.Rejudgements), len(tc.wantRejudgements))
			}
			for i, wantGeneration := range tc.wantRejudgements {
				got := snap.Rejudgements[i]
				if got.Generation != wantGeneration {
					t.Fatalf("rejudgement[%d].generation = %d, want %d", i, got.Generation, wantGeneration)
				}
				if got.Reason != arbiter.ReasonSuspectPositive {
					t.Fatalf("rejudgement[%d].reason = %s, want %s", i, got.Reason, arbiter.ReasonSuspectPositive)
				}
			}

			var gotAudit []inspection.Generation
			for _, ev := range snap.Audit {
				if ev.EventType == inspection.EventRejudged {
					gotAudit = append(gotAudit, ev.Generation)
				}
			}
			if len(gotAudit) != len(tc.wantRejudgedAudit) {
				t.Fatalf("rejudged audit count = %d, want %d", len(gotAudit), len(tc.wantRejudgedAudit))
			}
			for i, wantGeneration := range tc.wantRejudgedAudit {
				if gotAudit[i] != wantGeneration {
					t.Fatalf("rejudged audit[%d] generation = %d, want %d", i, gotAudit[i], wantGeneration)
				}
			}
		})
	}
}
