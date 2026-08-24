package evidence

import (
	"errors"
	"testing"
)

func TestMemoryWriterImmutability(t *testing.T) {
	w := NewMemoryWriter()
	rec := EvidenceRecord{
		TaskID: "t1", BlindCode: "BC1", Type: EvidenceAntibiotic,
		Raw: FixedPoint{Value: 190, Scale: 1}, Generation: 1,
	}
	if err := w.Write(rec); err != nil {
		t.Fatal(err)
	}
	// Same target at the same generation cannot be overwritten.
	if err := w.Write(rec); !errors.Is(err, ErrImmutable) {
		t.Fatalf("err = %v, want ErrImmutable", err)
	}
	// A newer generation is allowed (superseding rejudgement).
	rec.Generation = 2
	if err := w.Write(rec); err != nil {
		t.Fatalf("newer generation write failed: %v", err)
	}
	if got := w.Records(); len(got) != 2 {
		t.Fatalf("records = %d, want 2", len(got))
	}
}

func TestMemoryWriterInvalidRecord(t *testing.T) {
	w := NewMemoryWriter()
	if err := w.Write(EvidenceRecord{TaskID: "", Type: EvidenceAntibiotic}); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("err = %v, want ErrInvalidEvidence", err)
	}
}
