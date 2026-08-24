package store_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_SQLiteWithTxContextCancellationWhileQueued(t *testing.T) {
	openStore := func(t *testing.T) *store.SQLiteStore {
		t.Helper()
		st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "store.db"), catalog.NewFixedCatalog())
		if err != nil {
			t.Fatalf("open sqlite store: %v", err)
		}
		return st
	}

	task := func(id string) inspection.Task {
		return inspection.Task{
			ID:          inspection.TaskID(id),
			FarmID:      catalog.FixedFarmID,
			TankBatch:   inspection.TankBatch("batch-" + id),
			RuleVersion: catalog.FixedRuleVersion,
			Generation:  1,
			Status:      inspection.StatusPendingBuild,
			CreatedAt:   1,
		}
	}

	startHoldingTx := func(t *testing.T, st *store.SQLiteStore) (func(), <-chan error) {
		t.Helper()
		entered := make(chan struct{})
		release := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- st.WithTx(context.Background(), func(tx store.Tx) error {
				close(entered)
				<-release
				return nil
			})
		}()
		select {
		case <-entered:
		case err := <-done:
			t.Fatalf("holding transaction ended before blocking: %v", err)
		case <-time.After(time.Second):
			t.Fatal("holding transaction did not start")
		}
		var once sync.Once
		return func() { once.Do(func() { close(release) }) }, done
	}

	finishHoldingTx := func(t *testing.T, release func(), done <-chan error) {
		t.Helper()
		release()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("holding transaction: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("holding transaction did not finish")
		}
	}

	runCanceledWaiter := func(t *testing.T, makeContext func() (context.Context, context.CancelFunc), wantErr error) {
		t.Helper()
		st := openStore(t)
		defer st.Close()

		release, holderDone := startHoldingTx(t, st)
		var finishOnce sync.Once
		finishHolder := func() {
			finishOnce.Do(func() { finishHoldingTx(t, release, holderDone) })
		}
		defer finishHolder()

		ctx, cancel := makeContext()
		defer cancel()

		callbackRan := make(chan struct{})
		result := make(chan error, 1)
		go func() {
			result <- st.WithTx(ctx, func(tx store.Tx) error {
				close(callbackRan)
				return nil
			})
		}()

		select {
		case err := <-result:
			if !errors.Is(err, wantErr) {
				t.Fatalf("WithTx error = %v, want %v", err, wantErr)
			}
			select {
			case <-callbackRan:
				t.Fatal("transaction callback ran for a canceled waiter")
			default:
			}
		case <-time.After(500 * time.Millisecond):
			finishHolder()
			select {
			case err := <-result:
				t.Fatalf("WithTx waited for the prior writer to finish; returned after release with %v", err)
			case <-time.After(time.Second):
				t.Fatal("WithTx remained blocked after the prior writer was released")
			}
		}
	}

	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "already canceled context does not wait for the write transaction lock",
			run: func(t *testing.T) {
				runCanceledWaiter(t, func() (context.Context, context.CancelFunc) {
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					return ctx, cancel
				}, context.Canceled)
			},
		},
		{
			name: "deadline exceeded while queued does not wait for the holder to release",
			run: func(t *testing.T) {
				runCanceledWaiter(t, func() (context.Context, context.CancelFunc) {
					return context.WithTimeout(context.Background(), 20*time.Millisecond)
				}, context.DeadlineExceeded)
			},
		},
		{
			name: "uncanceled write commands remain serialized and commit",
			run: func(t *testing.T) {
				st := openStore(t)
				defer st.Close()

				release, holderDone := startHoldingTx(t, st)
				var finishOnce sync.Once
				finishHolder := func() {
					finishOnce.Do(func() { finishHoldingTx(t, release, holderDone) })
				}
				defer finishHolder()

				callbackStarted := make(chan struct{})
				result := make(chan error, 1)
				go func() {
					result <- st.WithTx(context.Background(), func(tx store.Tx) error {
						close(callbackStarted)
						return tx.CreateTask(context.Background(), task("serialized-commit"))
					})
				}()

				select {
				case <-callbackStarted:
					t.Fatal("queued write callback ran before the prior write transaction released")
				case err := <-result:
					t.Fatalf("queued write returned before the prior write transaction released: %v", err)
				case <-time.After(50 * time.Millisecond):
				}

				finishHolder()
				select {
				case err := <-result:
					if err != nil {
						t.Fatalf("queued write: %v", err)
					}
				case <-time.After(time.Second):
					t.Fatal("queued write did not finish after the prior transaction released")
				}

				got, err := st.GetTask(context.Background(), "serialized-commit")
				if err != nil {
					t.Fatalf("get committed task: %v", err)
				}
				if got.ID != "serialized-commit" {
					t.Fatalf("committed task ID = %q", got.ID)
				}
			},
		},
		{
			name: "callback error rolls back and is returned",
			run: func(t *testing.T) {
				st := openStore(t)
				defer st.Close()

				validationErr := errors.New("business validation failed")
				err := st.WithTx(context.Background(), func(tx store.Tx) error {
					if err := tx.CreateTask(context.Background(), task("rolled-back")); err != nil {
						return fmt.Errorf("create inside rollback case: %w", err)
					}
					return validationErr
				})
				if !errors.Is(err, validationErr) {
					t.Fatalf("WithTx error = %v, want validation error", err)
				}
				if _, err := st.GetTask(context.Background(), "rolled-back"); !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("rolled-back task lookup error = %v, want ErrNotFound", err)
				}
			},
		},
		{
			name: "closed store returns ErrClosed without running callback",
			run: func(t *testing.T) {
				st := openStore(t)
				if err := st.Close(); err != nil {
					t.Fatalf("close store: %v", err)
				}

				callbackRan := false
				err := st.WithTx(context.Background(), func(tx store.Tx) error {
					callbackRan = true
					return nil
				})
				if !errors.Is(err, store.ErrClosed) {
					t.Fatalf("WithTx error = %v, want ErrClosed", err)
				}
				if callbackRan {
					t.Fatal("callback ran after store was closed")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, tc.run)
	}
}
