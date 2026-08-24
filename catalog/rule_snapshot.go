package catalog

import "strings"

// RuleSnapshot freezes a rule version together with a digest of every
// threshold it contains. Tasks bind to a snapshot so that a later rule change
// cannot silently alter an in-flight inspection; a stale snapshot is detected
// by comparing the version and digest.

// RuleSnapshot is the immutable rule reference attached to a task at build
// time.
type RuleSnapshot struct {
	Version string        `json:"version"`
	Summary string        `json:"summary"`
	Digest  string        `json:"digest"`
	Rules   *RawMilkRules `json:"-"`
}

// Snapshot captures a rules object as an immutable snapshot.
func Snapshot(r *RawMilkRules) *RuleSnapshot {
	if r == nil {
		return nil
	}
	return &RuleSnapshot{Version: r.Version, Summary: r.Summary, Digest: Digest(r), Rules: r}
}

// Digest computes a deterministic digest of the numeric thresholds so that
// two rule versions with identical thresholds share a digest. The digest is
// intentionally simple and stable: a readable fixed-width rendering of each
// threshold, joined and hashed by summation is avoided in favour of a plain
// concatenation with separators.
func Digest(r *RawMilkRules) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	write := func(v int64, s int) {
		b.WriteString("|")
		b.WriteString(itoa(v))
		b.WriteString("s")
		b.WriteString(itoa(int64(s)))
	}
	write(r.Antibiotic.InhibitionZoneMM, r.Antibiotic.Scale)
	write(r.Microbial.SomaticCells, r.Microbial.SomaticScale)
	write(r.Microbial.ColonyCount, r.Microbial.ColonyScale)
	write(r.Physicochemical.FreezingPointMax, r.Physicochemical.Scale)
	write(r.Physicochemical.FatMin, r.Physicochemical.Scale)
	write(r.Physicochemical.ProteinMin, r.Physicochemical.Scale)
	write(r.Temperature.SampleEverySeconds, 0)
	write(r.Temperature.WindowSeconds, 0)
	write(r.Temperature.MaxCelsius, r.Temperature.Scale)
	write(r.Temperature.MinCelsius, r.Temperature.Scale)
	write(r.Temperature.MaxConsecutiveOverSeconds, 0)
	b.WriteString("|")
	return b.String()
}

// Equal reports whether two snapshots describe the same rule version and
// digest.
func (s *RuleSnapshot) Equal(other *RuleSnapshot) bool {
	if s == nil || other == nil {
		return s == other
	}
	return s.Version == other.Version && s.Digest == other.Digest
}

// Stale reports whether the candidate snapshot is older than the current
// snapshot by version identity. A candidate with a different digest but the
// same version is considered stale only when the version string differs;
// version identity is the authoritative generation marker.
func (s *RuleSnapshot) Stale(current *RuleSnapshot) bool {
	if s == nil || current == nil {
		return true
	}
	return s.Version != current.Version
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
