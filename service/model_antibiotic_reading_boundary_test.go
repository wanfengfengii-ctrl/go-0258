package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_AntibioticThresholdFailureIsPersistedForArbitration(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name             string
		value            string
		wantFault        string
		wantPass         bool
		wantStatus       inspection.Status
		wantEvidence     int
		wantAudit        int
		wantIdempotency  bool
		wantReportFail   bool
		wantFinalArbiter bool
	}{
		{
			name:             "legal_value_below_threshold_is_evidence_not_arithmetic_error",
			value:            "10.0",
			wantPass:         false,
			wantStatus:       inspection.StatusMicrobialCulturing,
			wantEvidence:     1,
			wantAudit:        1,
			wantIdempotency:  true,
			wantReportFail:   true,
			wantFinalArbiter: true,
		},
		{
			name:       "malformed_value_is_arithmetic_error_without_side_effects",
			value:      "10.01",
			wantFault:  CodeArithmeticFailure,
			wantStatus: inspection.StatusAntibioticReading,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, st := newFixtureService(t)
			defer st.Close()
			id := createFixtureTask(t, svc)
			confirmSampling(t, svc, id)
			splitBlind(t, svc, id)
			occupyResources(t, svc, id)
			writeColdChain(t, svc, id)

			_, fault := svc.SubmitReading(ctx, id, ReadingRequest{
				OperationID: "op-antibiotic-primer",
				Generation:  1,
				Type:        evidence.EvidenceAntibiotic,
				BlindCode:   "BCODE-A",
				Compartment: "A",
				Well:        "A1",
				Value:       "20.0",
			})
			if fault != nil {
				t.Fatalf("primer antibiotic reading: %v", fault)
			}

			before, fault := svc.GetSnapshot(ctx, id)
			if fault != nil {
				t.Fatalf("snapshot before boundary reading: %v", fault)
			}

			result, fault := svc.SubmitReading(ctx, id, ReadingRequest{
				OperationID: "op-antibiotic-boundary",
				Generation:  1,
				Type:        evidence.EvidenceAntibiotic,
				BlindCode:   "BCODE-B",
				Compartment: "B",
				Well:        "B1",
				Value:       tc.value,
			})
			if tc.wantFault != "" {
				if fault == nil || fault.Code != tc.wantFault {
					t.Fatalf("fault = %v, want %s", fault, tc.wantFault)
				}
			} else if fault != nil {
				t.Fatalf("boundary antibiotic reading rejected: %v", fault)
			}
			if fault == nil {
				if result.Pass != tc.wantPass {
					t.Fatalf("pass = %v, want %v", result.Pass, tc.wantPass)
				}
				if result.Status != tc.wantStatus {
					t.Fatalf("result status = %s, want %s", result.Status, tc.wantStatus)
				}
			}

			after, fault := svc.GetSnapshot(ctx, id)
			if fault != nil {
				t.Fatalf("snapshot after boundary reading: %v", fault)
			}
			if after.Task.Status != tc.wantStatus {
				t.Fatalf("task status = %s, want %s", after.Task.Status, tc.wantStatus)
			}
			if got, want := len(after.Evidence)-len(before.Evidence), tc.wantEvidence; got != want {
				t.Fatalf("evidence delta = %d, want %d", got, want)
			}
			if got, want := len(after.Audit)-len(before.Audit), tc.wantAudit; got != want {
				t.Fatalf("audit delta = %d, want %d", got, want)
			}

			var foundEvidence bool
			for _, rec := range after.Evidence {
				if rec.Type != evidence.EvidenceAntibiotic || rec.BlindCode != "BCODE-B" {
					continue
				}
				foundEvidence = true
				if rec.Raw.String() != tc.value {
					t.Fatalf("persisted value = %s, want %s", rec.Raw.String(), tc.value)
				}
				if !rec.Immutable || rec.Generation != 1 {
					t.Fatalf("evidence is not immutable generation 1: %+v", rec)
				}
			}
			if foundEvidence != tc.wantReportFail {
				t.Fatalf("boundary evidence found = %v, want %v", foundEvidence, tc.wantReportFail)
			}

			var foundAudit bool
			for _, ev := range after.Audit {
				if ev.EventType == inspection.EventReading && ev.Detail == "antibiotic="+tc.value {
					foundAudit = true
				}
			}
			if foundAudit != tc.wantReportFail {
				t.Fatalf("reading audit found = %v, want %v", foundAudit, tc.wantReportFail)
			}

			var rec inspection.IdempotencyRecord
			var exists bool
			err := st.WithTx(ctx, func(tx store.Tx) error {
				var err error
				rec, exists, err = tx.GetIdempotency(ctx, id, "op-antibiotic-boundary")
				return err
			})
			if err != nil {
				t.Fatalf("get idempotency: %v", err)
			}
			if exists != tc.wantIdempotency {
				t.Fatalf("idempotency exists = %v, want %v", exists, tc.wantIdempotency)
			}
			if exists {
				if rec.OperationType != inspection.OpReading || rec.ErrorCode != "" {
					t.Fatalf("idempotency record = %+v, want successful reading record", rec)
				}
			}

			report, fault := svc.BuildReport(ctx, id)
			if fault != nil {
				t.Fatalf("report: %v", fault)
			}
			var reportFailure bool
			for _, reading := range report.Readings {
				if reading.Type == evidence.EvidenceAntibiotic && reading.Blind == "BCODE-B" {
					if reading.Value != tc.value {
						t.Fatalf("report value = %s, want %s", reading.Value, tc.value)
					}
					if reading.Pass {
						t.Fatalf("report reading pass = true, want false")
					}
					reportFailure = true
				}
			}
			if reportFailure != tc.wantReportFail {
				t.Fatalf("report failure reading found = %v, want %v", reportFailure, tc.wantReportFail)
			}

			if !tc.wantFinalArbiter {
				return
			}
			for i, code := range []string{"BCODE-A", "BCODE-B"} {
				mustRead(t, svc, id, "op-som-"+strconvItoa(i), evidence.EvidenceSomaticCell, code, "350")
				mustRead(t, svc, id, "op-col-"+strconvItoa(i), evidence.EvidenceColony, code, "50000")
			}
			for i, code := range []string{"BCODE-A", "BCODE-B"} {
				mustRead(t, svc, id, "op-fp-"+strconvItoa(i), evidence.EvidenceFreezingPoint, code, "-53.0")
				mustRead(t, svc, id, "op-fat-"+strconvItoa(i), evidence.EvidenceFat, code, "3.5")
				mustRead(t, svc, id, "op-prot-"+strconvItoa(i), evidence.EvidenceProtein, code, "3.1")
			}
			passReviews(t, svc, id)

			_, fault = svc.Finalize(ctx, id, FinalizeRequest{
				OperationID: "op-final-admissible",
				Generation:  1,
				Outcome:     inspection.FinalAdmissible,
			})
			if fault == nil || fault.Code != CodeFinalizeConflict {
				t.Fatalf("admissible finalize fault = %v, want %s", fault, CodeFinalizeConflict)
			}
			if len(fault.Reasons) != 1 || fault.Reasons[0] != "antibiotic_suspect" {
				t.Fatalf("admissible finalize reasons = %v, want [antibiotic_suspect]", fault.Reasons)
			}

			final, fault := svc.Finalize(ctx, id, FinalizeRequest{
				OperationID: "op-final-quarantined",
				Generation:  1,
				Outcome:     inspection.FinalQuarantined,
			})
			if fault != nil {
				t.Fatalf("quarantine finalize: %v", fault)
			}
			if final.FinalType != inspection.FinalQuarantined {
				t.Fatalf("final type = %s, want %s", final.FinalType, inspection.FinalQuarantined)
			}
		})
	}
}
