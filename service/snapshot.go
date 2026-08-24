package service

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// GetSnapshot returns the full persisted state of a task.
func (s *Service) GetSnapshot(ctx context.Context, id inspection.TaskID) (*store.Snapshot, *Fault) {
	snap, err := s.store.Snapshot(ctx, id)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, NewFault(CodeNotFound, string(id))
		}
		return nil, NewFault(CodeStoreError, err.Error())
	}
	return snap, nil
}

// ListTasks returns all tasks ordered by creation time.
func (s *Service) ListTasks(ctx context.Context) ([]inspection.Task, *Fault) {
	tasks, err := s.store.ListTasks(ctx)
	if err != nil {
		return nil, NewFault(CodeStoreError, err.Error())
	}
	return tasks, nil
}
