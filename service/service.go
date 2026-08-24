package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// Service implements every business command over a durable Store. It owns the
// domain rules and delegates persistence to the store, so all commands are
// atomic and replay-safe.
type Service struct {
	store store.Store
	clock Clock
}

// NewService builds a service over the given store.
func NewService(s store.Store, clock Clock) *Service {
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: s, clock: clock}
}

// Store exposes the underlying store (used by the API for list/health).
func (s *Service) Store() store.Store { return s.store }

// Clock returns the service clock.
func (s *Service) Clock() Clock { return s.clock }

// Catalog returns the rule directory.
func (s *Service) Catalog() catalog.Catalog { return s.store.Catalog() }

// NewID generates a random hex identifier for tasks and credentials.
func NewID(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return prefix + "-" + hex.EncodeToString(b[:])
}

// ensureTaskLoaded is a helper that re-reads a task inside a transaction and
// returns a Fault when missing.
func (s *Service) taskFrom(ctx context.Context, tx store.Tx, id inspection.TaskID) (inspection.Task, *Fault) {
	t, err := tx.GetTask(ctx, id)
	if err != nil {
		if err == store.ErrNotFound {
			return inspection.Task{}, NewFault(CodeNotFound, string(id))
		}
		return inspection.Task{}, NewFault(CodeStoreError, err.Error())
	}
	return t, nil
}
