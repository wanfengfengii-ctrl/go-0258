package occupancy

import (
	"sync"
	"testing"
)

// TestLedgerConcurrentAcquireSingleWinner races two tasks for the same plate
// well and asserts exactly one wins and the loser has no residual lease.
func TestLedgerConcurrentAcquireSingleWinner(t *testing.T) {
	ledger := NewMemoryLedger()
	well := func(taskID string) Occupancy {
		return Occupancy{
			TaskID: taskID, ResourceType: ResourcePlateWell,
			PlateID: "P1", Well: "A1", StartAt: 0, EndAt: 100, Generation: 1,
		}
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = ledger.Acquire(well("task-" + string(rune('a'+i))))
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	for _, e := range errs {
		if e == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want 1 (errs=%v)", winners, errs)
	}
	if len(ledger.ActiveHeldBy("task-a"))+len(ledger.ActiveHeldBy("task-b")) != 1 {
		t.Fatal("expected exactly one active lease across both tasks")
	}
}

// TestLedgerIncubatorIntervalConflict asserts overlapping incubator intervals
// conflict while disjoint ones do not.
func TestLedgerIncubatorIntervalConflict(t *testing.T) {
	ledger := NewMemoryLedger()
	a := Occupancy{TaskID: "t1", ResourceType: ResourceIncubator, IncubatorID: "inc-1", StartAt: 0, EndAt: 100, Generation: 1}
	b := Occupancy{TaskID: "t2", ResourceType: ResourceIncubator, IncubatorID: "inc-1", StartAt: 50, EndAt: 150, Generation: 1}
	c := Occupancy{TaskID: "t3", ResourceType: ResourceIncubator, IncubatorID: "inc-1", StartAt: 100, EndAt: 200, Generation: 1}

	if _, err := ledger.Acquire(a); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Acquire(b); err != ErrOccupied {
		t.Fatalf("err = %v, want ErrOccupied", err)
	}
	// c is disjoint (starts exactly at a's end), so it must succeed.
	if _, err := ledger.Acquire(c); err != nil {
		t.Fatalf("disjoint interval rejected: %v", err)
	}
}

// TestParseWell validates the plate-well coordinate grammar.
func TestParseWell(t *testing.T) {
	valid := []string{"A1", "H12", "c3", "B10"}
	for _, w := range valid {
		if _, ok := ParseWell(w); !ok {
			t.Fatalf("well %q should be valid", w)
		}
	}
	invalid := []string{"", "A", "A0", "A13", "I1", "1A", "A-1"}
	for _, w := range invalid {
		if _, ok := ParseWell(w); ok {
			t.Fatalf("well %q should be invalid", w)
		}
	}
}

// TestOccupancyValidate checks lease validation.
func TestOccupancyValidate(t *testing.T) {
	ok := Occupancy{TaskID: "t", ResourceType: ResourcePlateWell, PlateID: "P", Well: "A1", StartAt: 0, EndAt: 10}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid lease rejected: %v", err)
	}
	bad := Occupancy{TaskID: "t", ResourceType: ResourcePlateWell, PlateID: "P", Well: "A1", StartAt: 10, EndAt: 10}
	if err := bad.Validate(); err == nil {
		t.Fatal("empty interval should be invalid")
	}
	missing := Occupancy{TaskID: "", ResourceType: ResourceIncubator, IncubatorID: "i", StartAt: 0, EndAt: 10}
	if err := missing.Validate(); err == nil {
		t.Fatal("missing task id should be invalid")
	}
}
