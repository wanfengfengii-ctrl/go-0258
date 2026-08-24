package occupancy

import (
	"fmt"
	"strings"
)

// Plate-well addressing. An inhibition plate is addressed by a plate id and a
// well coordinate such as "A1".."H12". The ledger validates coordinates so a
// malformed well can never reserve a resource.

// Well describes a single inhibition-plate well coordinate.
type Well string

// ParseWell validates a well coordinate of the form <row letter><column
// number>. Rows run A..H and columns 1..12 (a 96-well plate). It returns the
// normalized well and a boolean.
func ParseWell(s string) (Well, bool) {
	if len(s) < 2 || len(s) > 3 {
		return "", false
	}
	row := s[0]
	if row >= 'a' && row <= 'z' {
		row = row - 'a' + 'A'
	}
	if row < 'A' || row > 'H' {
		return "", false
	}
	col := 0
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return "", false
		}
		col = col*10 + int(c-'0')
	}
	if col < 1 || col > 12 {
		return "", false
	}
	return Well(strings.ToUpper(s)), true
}

// PlateWell builds a plate-well occupancy after validating the well. It
// returns ErrInvalidLease when the well is malformed or the interval is
// empty.
func PlateWell(taskID, plateID, well string, startAt, endAt, generation int64) (Occupancy, error) {
	w, ok := ParseWell(well)
	if !ok {
		return Occupancy{}, fmt.Errorf("%w: malformed well %q", ErrInvalidLease, well)
	}
	o := Occupancy{
		TaskID:       taskID,
		ResourceType: ResourcePlateWell,
		PlateID:      plateID,
		Well:         string(w),
		StartAt:      startAt,
		EndAt:        endAt,
		Generation:   generation,
	}
	if err := o.Validate(); err != nil {
		return Occupancy{}, err
	}
	return o, nil
}

// WellColumns returns the 1-based column number of a well coordinate.
func (w Well) WellColumns() int {
	if _, ok := ParseWell(string(w)); !ok {
		return 0
	}
	col := 0
	for i := 1; i < len(w); i++ {
		col = col*10 + int(w[i]-'0')
	}
	return col
}
