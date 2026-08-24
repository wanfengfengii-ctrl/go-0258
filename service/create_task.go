package service

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// CreateTaskRequest freezes every rule and resource candidate at build time.
type CreateTaskRequest struct {
	TaskID        inspection.TaskID         `json:"taskId"`
	FarmID        catalog.FarmID            `json:"farmId"`
	TankBatch     inspection.TankBatch      `json:"tankBatch"`
	Compartments  []catalog.CompartmentCode `json:"compartments"`
	Seals         []catalog.SealCode        `json:"seals"`
	RecorderModel catalog.RecorderModel     `json:"recorderModel"`
	RuleVersion   string                    `json:"ruleVersion"`
	Reviewers     []catalog.PersonID        `json:"reviewers"`
}

// CreateTaskResult is the outcome of a build: the task with its initial
// generation and an occupancy summary.
type CreateTaskResult struct {
	Task       inspection.Task       `json:"task"`
	Generation inspection.Generation `json:"generation"`
	Status     inspection.Status     `json:"status"`
}

// CreateTask builds a tank-batch inspection task and freezes the farm,
// compartments, seals, recorder, thresholds and reviewers in one transaction.
func (s *Service) CreateTask(ctx context.Context, req CreateTaskRequest) (*CreateTaskResult, *Fault) {
	if req.TaskID == "" || req.FarmID == "" || req.TankBatch == "" {
		return nil, NewFault(CodeBadRequest, "taskId, farmId and tankBatch are required")
	}

	farm, ok := s.Catalog().Farm(req.FarmID)
	if !ok {
		return nil, NewFault(CodeUnknownFarm, string(req.FarmID))
	}
	rules, ok := s.Catalog().Rules(req.RuleVersion)
	if !ok {
		return nil, NewFault(CodeUnknownRules, req.RuleVersion)
	}
	// Stale rule rejection: the farm's locked rule version must match.
	if farm.RuleVersion != "" && farm.RuleVersion != req.RuleVersion {
		return nil, NewFault(CodeStaleRules, "farm rules "+farm.RuleVersion+" vs requested "+req.RuleVersion)
	}
	// Validate compartments and seal scope.
	if v := catalog.ValidateSealScope(farm, req.Compartments, req.Seals); v != nil {
		return nil, NewFault(codeForSeal(v.Code), v.Message)
	}
	// Reviewers must be distinct and qualified under the rule version.
	if err := validateReviewerRoster(s.Catalog(), req.RuleVersion, rules.RequiredReviewers, req.Reviewers); err != nil {
		return nil, err
	}

	ctx = context.Background()
	now := s.clock.Now()
	task := inspection.Task{
		ID:            req.TaskID,
		FarmID:        req.FarmID,
		TankBatch:     req.TankBatch,
		Compartments:  append([]catalog.CompartmentCode(nil), req.Compartments...),
		Seals:         append([]catalog.SealCode(nil), req.Seals...),
		RecorderModel: req.RecorderModel,
		RuleVersion:   req.RuleVersion,
		Generation:    1,
		Status:        inspection.StatusPendingSampling,
		CreatedAt:     now,
		Reviewers:     append([]catalog.PersonID(nil), req.Reviewers...),
	}

	err := s.store.WithTx(ctx, func(tx store.Tx) error {
		if err := tx.CreateTask(ctx, task); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, inspection.AuditEvent{
			TaskID:      task.ID,
			Generation:  task.Generation,
			EventType:   inspection.EventTaskCreated,
			LogicalTime: now,
			Detail:      "built tank batch " + string(task.TankBatch),
		})
	})
	if err != nil {
		if err == store.ErrConflict {
			return nil, NewFault(CodeConflict, "task already exists")
		}
		return nil, NewFault(CodeStoreError, err.Error())
	}

	return &CreateTaskResult{Task: task, Generation: task.Generation, Status: task.Status}, nil
}

func codeForSeal(code string) string {
	switch code {
	case "unknown_farm":
		return CodeUnknownFarm
	case "unknown_seal", "seal_scope_gap", "duplicate_seal":
		return CodeBadRequest
	default:
		return CodeBadRequest
	}
}

// validateReviewerRoster checks the reviewer list is non-empty, distinct, and
// every reviewer is qualified under the rule version.
func validateReviewerRoster(cat catalog.Catalog, version string, required int, reviewers []catalog.PersonID) *Fault {
	if len(reviewers) < required {
		return NewFault(CodeNotQualified, "requires at least "+itoa(required)+" reviewers")
	}
	seen := map[catalog.PersonID]bool{}
	for _, id := range reviewers {
		if seen[id] {
			return NewFault(CodeRoleOverlap, "duplicate reviewer "+string(id))
		}
		seen[id] = true
		if !cat.Qualifies(version, id, catalog.RoleReviewer) {
			return NewFault(CodeNotQualified, "reviewer "+string(id)+" not qualified")
		}
	}
	return nil
}

func itoa(n int) string {
	return strconvItoa(n)
}
