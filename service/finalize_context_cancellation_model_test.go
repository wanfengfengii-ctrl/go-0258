package service

import (
	"context"
	"errors"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

type finalizeCancelPoint string

const (
	finalizeCancelBeforeTaskUpdate    finalizeCancelPoint = "before task update"
	finalizeCancelBeforeFinalDecision finalizeCancelPoint = "before final decision"
	finalizeCancelBeforeCommit        finalizeCancelPoint = "before commit"
)

type finalizeCancelStore struct {
	store.Store
	cancel    context.CancelFunc
	at        finalizeCancelPoint
	armed     bool
	cancelled bool
}

func (s *finalizeCancelStore) WithTx(ctx context.Context, fn func(store.Tx) error) error {
	return s.Store.WithTx(ctx, func(tx store.Tx) error {
		if err := fn(&finalizeCancelTx{Tx: tx, parent: s}); err != nil {
			return err
		}
		if s.armed && s.at == finalizeCancelBeforeCommit {
			s.cancelled = true
			s.cancel()
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		return nil
	})
}

type finalizeCancelTx struct {
	store.Tx
	parent *finalizeCancelStore
}

func (tx *finalizeCancelTx) UpdateTaskCAS(ctx context.Context, id inspection.TaskID, wantStatus inspection.Status, wantGeneration inspection.Generation, update inspection.Task) error {
	if tx.parent.armed && tx.parent.at == finalizeCancelBeforeTaskUpdate {
		tx.parent.cancelled = true
		tx.parent.cancel()
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return tx.Tx.UpdateTaskCAS(ctx, id, wantStatus, wantGeneration, update)
}

func (tx *finalizeCancelTx) PutFinalDecision(ctx context.Context, taskID inspection.TaskID, finalType inspection.FinalType, credential string, logicalTime int64) error {
	if tx.parent.armed && tx.parent.at == finalizeCancelBeforeFinalDecision {
		tx.parent.cancelled = true
		tx.parent.cancel()
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return tx.Tx.PutFinalDecision(ctx, taskID, finalType, credential, logicalTime)
}

func TestModel_FinalizeCancellationPreventsTerminalWrites(t *testing.T) {
	cases := []struct {
		name    string
		outcome inspection.FinalType
		cancel  finalizeCancelPoint
		setup   func(t *testing.T, svc *Service, id inspection.TaskID)
	}{
		{
			name:    "admissible cancelled before task status update",
			outcome: inspection.FinalAdmissible,
			cancel:  finalizeCancelBeforeTaskUpdate,
			setup: func(t *testing.T, svc *Service, id inspection.TaskID) {
				advanceToReview(t, svc, id)
				passReviews(t, svc, id)
			},
		},
		{
			name:    "admissible cancelled before final decision insert",
			outcome: inspection.FinalAdmissible,
			cancel:  finalizeCancelBeforeFinalDecision,
			setup: func(t *testing.T, svc *Service, id inspection.TaskID) {
				advanceToReview(t, svc, id)
				passReviews(t, svc, id)
			},
		},
		{
			name:    "admissible cancelled before transaction commit",
			outcome: inspection.FinalAdmissible,
			cancel:  finalizeCancelBeforeCommit,
			setup: func(t *testing.T, svc *Service, id inspection.TaskID) {
				advanceToReview(t, svc, id)
				passReviews(t, svc, id)
			},
		},
		{
			name:    "quarantined cancelled before final decision insert",
			outcome: inspection.FinalQuarantined,
			cancel:  finalizeCancelBeforeFinalDecision,
			setup: func(t *testing.T, svc *Service, id inspection.TaskID) {
				advanceToReview(t, svc, id)
			},
		},
		{
			name:    "cancelled cancelled before final decision insert",
			outcome: inspection.FinalCancelled,
			cancel:  finalizeCancelBeforeFinalDecision,
			setup: func(t *testing.T, svc *Service, id inspection.TaskID) {
				advanceToReview(t, svc, id)
			},
		},
		{
			name:    "entered cancelled before task status update",
			outcome: inspection.FinalEntered,
			cancel:  finalizeCancelBeforeTaskUpdate,
			setup: func(t *testing.T, svc *Service, id inspection.TaskID) {
				advanceToReview(t, svc, id)
				passReviews(t, svc, id)
				if _, fault := svc.Finalize(context.Background(), id, FinalizeRequest{
					OperationID: "op-existing-admissible",
					Generation:  1,
					Outcome:     inspection.FinalAdmissible,
				}); fault != nil {
					t.Fatalf("baseline admissible finalize: %v", fault)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, st := newFixtureService(t)
			defer st.Close()
			id := createFixtureTask(t, svc)
			tc.setup(t, svc, id)

			before, fault := svc.GetSnapshot(context.Background(), id)
			if fault != nil {
				t.Fatalf("baseline snapshot: %v", fault)
			}

			ctx, cancel := context.WithCancel(context.Background())
			wrapped := &finalizeCancelStore{Store: st, cancel: cancel, at: tc.cancel, armed: true}
			cancelSvc := NewService(wrapped, svc.Clock())
			opID := inspection.OperationID("op-cancelled-finalize-" + string(tc.outcome) + "-" + string(tc.cancel))

			result, fault := cancelSvc.Finalize(ctx, id, FinalizeRequest{
				OperationID: opID,
				Generation:  1,
				Outcome:     tc.outcome,
			})
			if !wrapped.cancelled || !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("finalize did not reach cancellation point; cancelled=%v ctx=%v", wrapped.cancelled, ctx.Err())
			}
			if fault == nil {
				t.Fatalf("finalize unexpectedly succeeded after request cancellation: %+v", result)
			}

			after, fault := svc.GetSnapshot(context.Background(), id)
			if fault != nil {
				t.Fatalf("post-cancel snapshot: %v", fault)
			}
			if after.Task.Status != before.Task.Status {
				t.Fatalf("status = %s, want unchanged %s", after.Task.Status, before.Task.Status)
			}
			if after.Task.FinalType != before.Task.FinalType {
				t.Fatalf("task final type = %s, want unchanged %s", after.Task.FinalType, before.Task.FinalType)
			}
			if (after.FinalDecision == nil) != (before.FinalDecision == nil) {
				t.Fatalf("final decision changed from %+v to %+v", before.FinalDecision, after.FinalDecision)
			}
			if before.FinalDecision != nil && *after.FinalDecision != *before.FinalDecision {
				t.Fatalf("final decision = %+v, want unchanged %+v", after.FinalDecision, before.FinalDecision)
			}
			if len(after.Audit) != len(before.Audit) {
				t.Fatalf("audit event count = %d, want unchanged %d", len(after.Audit), len(before.Audit))
			}
			for _, ev := range after.Audit {
				if ev.EventType == inspection.EventFinalized && ev.Detail == string(tc.outcome) && tc.outcome != inspection.FinalEntered {
					t.Fatalf("cancelled finalize wrote audit event: %+v", ev)
				}
			}

			err := st.WithTx(context.Background(), func(tx store.Tx) error {
				_, exists, err := tx.GetIdempotency(context.Background(), id, opID)
				if err != nil {
					return err
				}
				if exists {
					return errors.New("cancelled finalize wrote an idempotency record")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
