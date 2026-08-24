package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func samplingReq(op string, person catalog.PersonID, batch string) SamplingConfirmationRequest {
	return SamplingConfirmationRequest{
		OperationID:  inspection.OperationID(op),
		Person:       person,
		FarmID:       catalog.FixedFarmID,
		TankBatch:    inspection.TankBatch(batch),
		Compartments: []catalog.CompartmentCode{"A", "B"},
		Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
		Generation:   1,
	}
}

func newSampledTask(t *testing.T) (*Service, *store.MemoryStore, inspection.TaskID) {
	t.Helper()
	svc, st := newFixtureService(t)
	id := createFixtureTask(t, svc)
	return svc, st, id
}

func TestSamplingConfirmIdempotentRetry(t *testing.T) {
	svc, st, id := newSampledTask(t)
	defer st.Close()

	first, fault := svc.SamplingConfirm(context.Background(), id, samplingReq("op-a", catalog.FixedSamplerA, "BATCH-2026-001"))
	if fault != nil {
		t.Fatal(fault)
	}
	// Identical retry must return the same result, not conflict.
	again, fault := svc.SamplingConfirm(context.Background(), id, samplingReq("op-a", catalog.FixedSamplerA, "BATCH-2026-001"))
	if fault != nil {
		t.Fatalf("idempotent retry failed: %v", fault)
	}
	if first.Status != again.Status || len(first.Confirmed) != len(again.Confirmed) {
		t.Fatalf("retry result differs: %+v vs %+v", first, again)
	}

	// A second, distinct operator completes dual confirmation and advances the
	// task to blind_splitting. The frontend then retries sampler A's original
	// request with the same operation id and content after a network blip.
	if _, fault := svc.SamplingConfirm(context.Background(), id, samplingReq("op-b", catalog.FixedSamplerB, "BATCH-2026-001")); fault != nil {
		t.Fatal(fault)
	}
	advanced, fault := svc.GetSnapshot(context.Background(), id)
	if fault != nil {
		t.Fatal(fault)
	}
	if advanced.Task.Status != inspection.StatusBlindSplitting {
		t.Fatalf("status = %s, want blind_splitting after dual confirmation", advanced.Task.Status)
	}
	// The retry of an already-applied confirmation must replay its result even
	// though the task has since left pending_sampling; it must not report
	// illegal_transition.
	retry, fault := svc.SamplingConfirm(context.Background(), id, samplingReq("op-a", catalog.FixedSamplerA, "BATCH-2026-001"))
	if fault != nil {
		t.Fatalf("idempotent retry after status advanced failed: %v", fault)
	}
	if retry.Status != inspection.StatusBlindSplitting || !retry.Complete {
		t.Fatalf("retry after advance = %+v, want blind_splitting complete", retry)
	}
}

func TestSamplingConfirmContentConflict(t *testing.T) {
	svc, st, id := newSampledTask(t)
	defer st.Close()

	if _, fault := svc.SamplingConfirm(context.Background(), id, samplingReq("op-a", catalog.FixedSamplerA, "BATCH-2026-001")); fault != nil {
		t.Fatal(fault)
	}
	// Same operation id with different content must conflict.
	_, fault := svc.SamplingConfirm(context.Background(), id, samplingReq("op-a", catalog.FixedSamplerA, "BATCH-2026-999"))
	if fault == nil || fault.Code != CodeContentConflict {
		t.Fatalf("fault = %v, want content_conflict", fault)
	}
}

func TestSamplingConfirmRoleOverlapRejected(t *testing.T) {
	svc, st, id := newSampledTask(t)
	defer st.Close()

	// A reviewer must not sample (role overlap).
	_, fault := svc.SamplingConfirm(context.Background(), id, samplingReq("op-r", catalog.FixedReviewerA, "BATCH-2026-001"))
	if fault == nil || fault.Code != CodeNotQualified {
		t.Fatalf("fault = %v, want not_qualified for reviewer sampling", fault)
	}
}

func TestSamplingConfirmStaleGeneration(t *testing.T) {
	svc, st, id := newSampledTask(t)
	defer st.Close()

	req := samplingReq("op-a", catalog.FixedSamplerA, "BATCH-2026-001")
	req.Generation = 99
	_, fault := svc.SamplingConfirm(context.Background(), id, req)
	if fault == nil || fault.Code != CodeStaleGeneration {
		t.Fatalf("fault = %v, want stale_generation", fault)
	}
}
