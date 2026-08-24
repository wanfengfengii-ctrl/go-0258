package inspection

import "testing"

func TestStatusPipelineAdvance(t *testing.T) {
	want := []Status{
		StatusPendingBuild, StatusPendingSampling, StatusBlindSplitting,
		StatusPlateOccupied, StatusColdChainVerifying, StatusAntibioticReading,
		StatusMicrobialCulturing, StatusPhysicochemical, StatusPendingReview,
	}
	for i := 0; i < len(want)-1; i++ {
		if !want[i].CanAdvanceTo(want[i+1]) {
			t.Fatalf("%s should advance to %s", want[i], want[i+1])
		}
	}
}

func TestStatusTerminalOnlyFromPendingReview(t *testing.T) {
	for _, terminal := range []Status{StatusAdmissible, StatusEntered, StatusQuarantined, StatusCancelled} {
		if !StatusPendingReview.CanAdvanceTo(terminal) {
			t.Fatalf("pending_review should reach %s", terminal)
		}
		if StatusColdChainVerifying.CanAdvanceTo(terminal) {
			t.Fatalf("cold_chain_verifying must not reach %s directly", terminal)
		}
	}
}

func TestTerminalNeverAdvances(t *testing.T) {
	for _, terminal := range []Status{StatusAdmissible, StatusEntered, StatusQuarantined, StatusCancelled} {
		if !terminal.IsTerminal() {
			t.Fatalf("%s should be terminal", terminal)
		}
		if terminal.CanAdvanceTo(StatusEntered) {
			t.Fatalf("%s must not advance", terminal)
		}
	}
}

func TestTaskAdvanceUpdatesStatus(t *testing.T) {
	task := Task{Status: StatusPendingBuild}
	if !task.Advance(StatusPendingSampling) {
		t.Fatal("expected advance to succeed")
	}
	if task.Status != StatusPendingSampling {
		t.Fatalf("status = %s, want pending_sampling", task.Status)
	}
	if task.Advance(StatusQuarantined) {
		t.Fatal("illegal transition must be rejected")
	}
}
