package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

type createTaskContextProbeKey struct{}

type createTaskContextProbeStore struct {
	store.Store

	marker                  string
	cancelBeforePersistence context.CancelFunc

	withTxCalled         bool
	withTxHadMarker      bool
	createCalled         bool
	createHadMarker      bool
	createWasCanceled    bool
	listAuditCalled      bool
	listAuditHadMarker   bool
	appendAuditCalled    bool
	appendAuditHadMarker bool
}

func (s *createTaskContextProbeStore) WithTx(ctx context.Context, fn func(tx store.Tx) error) error {
	s.withTxCalled = true
	s.withTxHadMarker = s.hasMarker(ctx)
	return s.Store.WithTx(ctx, func(tx store.Tx) error {
		if s.cancelBeforePersistence != nil {
			cancel := s.cancelBeforePersistence
			s.cancelBeforePersistence = nil
			cancel()
		}
		return fn(&createTaskContextProbeTx{Tx: tx, store: s})
	})
}

func (s *createTaskContextProbeStore) hasMarker(ctx context.Context) bool {
	return ctx != nil && ctx.Value(createTaskContextProbeKey{}) == s.marker
}

type createTaskContextProbeTx struct {
	store.Tx
	store *createTaskContextProbeStore
}

func (tx *createTaskContextProbeTx) CreateTask(ctx context.Context, task inspection.Task) error {
	tx.store.createCalled = true
	tx.store.createHadMarker = tx.store.hasMarker(ctx)
	tx.store.createWasCanceled = ctx.Err() != nil
	return tx.Tx.CreateTask(ctx, task)
}

func (tx *createTaskContextProbeTx) ListAudit(ctx context.Context, taskID inspection.TaskID) ([]inspection.AuditEvent, error) {
	tx.store.listAuditCalled = true
	tx.store.listAuditHadMarker = tx.store.hasMarker(ctx)
	return tx.Tx.ListAudit(ctx, taskID)
}

func (tx *createTaskContextProbeTx) AppendAudit(ctx context.Context, ev inspection.AuditEvent) error {
	tx.store.appendAuditCalled = true
	tx.store.appendAuditHadMarker = tx.store.hasMarker(ctx)
	return tx.Tx.AppendAudit(ctx, ev)
}

func TestModel_CreateTaskContextCancellationPreventsPersistence(t *testing.T) {
	baseRequest := func(id inspection.TaskID) CreateTaskRequest {
		return CreateTaskRequest{
			TaskID:        id,
			FarmID:        catalog.FixedFarmID,
			TankBatch:     inspection.TankBatch("BATCH-" + string(id)),
			Compartments:  []catalog.CompartmentCode{"A", "B"},
			Seals:         []catalog.SealCode{"seal-0001", "seal-0002"},
			RecorderModel: "recorder-x1",
			RuleVersion:   catalog.FixedRuleVersion,
			Reviewers:     []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
		}
	}

	auditCount := func(t *testing.T, st store.Store, taskID inspection.TaskID) int {
		t.Helper()
		var events []inspection.AuditEvent
		err := st.WithTx(context.Background(), func(tx store.Tx) error {
			var err error
			events, err = tx.ListAudit(context.Background(), taskID)
			return err
		})
		if err != nil {
			t.Fatalf("list audit: %v", err)
		}
		return len(events)
	}

	tests := []struct {
		name                    string
		taskID                  inspection.TaskID
		seedExisting            bool
		cancelBeforePersistence bool
		mutate                  func(*CreateTaskRequest)
		wantFault               bool
		wantFaultCode           string
		wantTasks               int
		wantAudit               int
		wantWithTxCalled        bool
		wantCreateCalled        bool
		wantCreateCanceled      bool
		wantListAuditCalled     bool
		wantAppendAuditCalled   bool
	}{
		{
			name:                  "normal create persists task and audit",
			taskID:                "task-model-normal",
			wantTasks:             1,
			wantAudit:             1,
			wantWithTxCalled:      true,
			wantCreateCalled:      true,
			wantListAuditCalled:   true,
			wantAppendAuditCalled: true,
		},
		{
			name:                    "canceled create writes no task or audit",
			taskID:                  "task-model-canceled",
			cancelBeforePersistence: true,
			wantFault:               true,
			wantWithTxCalled:        true,
			wantCreateCalled:        true,
			wantCreateCanceled:      true,
		},
		{
			name:             "duplicate task id remains conflict",
			taskID:           "task-model-duplicate",
			seedExisting:     true,
			wantFault:        true,
			wantFaultCode:    CodeConflict,
			wantTasks:        1,
			wantAudit:        1,
			wantWithTxCalled: true,
			wantCreateCalled: true,
		},
		{
			name:          "duplicate seal remains bad request",
			taskID:        "task-model-seal",
			mutate:        func(req *CreateTaskRequest) { req.Seals = []catalog.SealCode{"seal-0001", "seal-0001"} },
			wantFault:     true,
			wantFaultCode: CodeBadRequest,
		},
		{
			name:   "unqualified reviewer remains rejected",
			taskID: "task-model-reviewer",
			mutate: func(req *CreateTaskRequest) {
				req.Reviewers = []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedSamplerA}
			},
			wantFault:     true,
			wantFaultCode: CodeNotQualified,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inner := store.NewMemoryStore(catalog.NewFixedCatalog())
			defer inner.Close()

			req := baseRequest(tc.taskID)
			if tc.mutate != nil {
				tc.mutate(&req)
			}

			if tc.seedExisting {
				seedSvc := NewService(inner, NewManualClock(900))
				if _, fault := seedSvc.CreateTask(context.Background(), baseRequest(tc.taskID)); fault != nil {
					t.Fatalf("seed create: %v", fault)
				}
			}

			marker := "caller-context-" + tc.name
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), createTaskContextProbeKey{}, marker))
			defer cancel()

			probed := &createTaskContextProbeStore{Store: inner, marker: marker}
			if tc.cancelBeforePersistence {
				probed.cancelBeforePersistence = cancel
			}
			svc := NewService(probed, NewManualClock(1000))

			result, fault := svc.CreateTask(ctx, req)
			if tc.wantFault {
				if fault == nil {
					t.Fatalf("fault = nil, want fault")
				}
				if tc.wantFaultCode != "" && fault.Code != tc.wantFaultCode {
					t.Fatalf("fault code = %q, want %q", fault.Code, tc.wantFaultCode)
				}
				if result != nil {
					t.Fatalf("result = %#v, want nil on fault", result)
				}
			} else if fault != nil {
				t.Fatalf("fault = %v, want nil", fault)
			}

			tasks, err := inner.ListTasks(context.Background())
			if err != nil {
				t.Fatalf("list tasks: %v", err)
			}
			if len(tasks) != tc.wantTasks {
				t.Fatalf("persisted task count = %d, want %d", len(tasks), tc.wantTasks)
			}
			if got := auditCount(t, inner, tc.taskID); got != tc.wantAudit {
				t.Fatalf("audit event count = %d, want %d", got, tc.wantAudit)
			}

			if probed.withTxCalled != tc.wantWithTxCalled {
				t.Fatalf("WithTx called = %v, want %v", probed.withTxCalled, tc.wantWithTxCalled)
			}
			if probed.withTxCalled && !probed.withTxHadMarker {
				t.Fatalf("WithTx did not receive the caller context")
			}
			if probed.createCalled != tc.wantCreateCalled {
				t.Fatalf("Tx.CreateTask called = %v, want %v", probed.createCalled, tc.wantCreateCalled)
			}
			if probed.createCalled && !probed.createHadMarker {
				t.Fatalf("Tx.CreateTask did not receive the caller context")
			}
			if probed.createCalled && probed.createWasCanceled != tc.wantCreateCanceled {
				t.Fatalf("Tx.CreateTask saw canceled context = %v, want %v", probed.createWasCanceled, tc.wantCreateCanceled)
			}
			if probed.listAuditCalled != tc.wantListAuditCalled {
				t.Fatalf("Tx.ListAudit called = %v, want %v", probed.listAuditCalled, tc.wantListAuditCalled)
			}
			if probed.listAuditCalled && !probed.listAuditHadMarker {
				t.Fatalf("Tx.ListAudit did not receive the caller context")
			}
			if probed.appendAuditCalled != tc.wantAppendAuditCalled {
				t.Fatalf("Tx.AppendAudit called = %v, want %v", probed.appendAuditCalled, tc.wantAppendAuditCalled)
			}
			if probed.appendAuditCalled && !probed.appendAuditHadMarker {
				t.Fatalf("Tx.AppendAudit did not receive the caller context")
			}
		})
	}
}
