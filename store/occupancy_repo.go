package store

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
)

func scanOccupancy(rows interface{ Scan(...any) error }) (occupancy.Occupancy, error) {
	var (
		o           occupancy.Occupancy
		resourceKey string
	)
	err := rows.Scan(&o.TaskID, &o.ResourceType, &resourceKey, &o.PlateID, &o.Well, &o.IncubatorID, &o.StartAt, &o.EndAt, &o.Generation, &o.ReleasedAt)
	if err != nil {
		return occupancy.Occupancy{}, err
	}
	return o, nil
}

const occupancyColumns = `task_id, resource_type, resource_key, plate_id, well, incubator_id, start_at, end_at, generation, released_at`

// AcquireOccupancy atomically reserves a lease. It first reads every active
// lease on the same resource key and rejects any time overlap; because writes
// are serialized, the winner is deterministic and the loser has no residual
// row.
func (s *sqliteTx) AcquireOccupancy(ctx context.Context, o occupancy.Occupancy) error {
	if err := o.Validate(); err != nil {
		return err
	}
	active, err := s.ActiveOccupancyFor(ctx, o)
	if err != nil {
		return err
	}
	for _, existing := range active {
		if o.Overlaps(&existing) {
			return occupancy.ErrOccupied
		}
	}
	_, err = s.tx.ExecContext(ctx,
		`INSERT INTO resource_occupancies (task_id, resource_type, resource_key, plate_id, well, incubator_id, start_at, end_at, generation, released_at)
		 VALUES (?,?,?,?,?,?,?,?,?,0)`,
		o.TaskID, o.ResourceType, o.ResourceKey(), o.PlateID, o.Well, o.IncubatorID, o.StartAt, o.EndAt, o.Generation,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return occupancy.ErrOccupied
		}
		return err
	}
	return nil
}

// ActiveOccupancyFor returns active (unreleased) leases on the same resource
// as o.
func (s *sqliteTx) ActiveOccupancyFor(ctx context.Context, o occupancy.Occupancy) ([]occupancy.Occupancy, error) {
	rows, err := s.tx.QueryContext(ctx,
		`SELECT `+occupancyColumns+` FROM resource_occupancies WHERE resource_key=? AND released_at=0`, o.ResourceKey())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []occupancy.Occupancy
	for rows.Next() {
		e, err := scanOccupancy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListOccupancy returns all leases for a task.
func (s *sqliteTx) ListOccupancy(ctx context.Context, taskID inspection.TaskID) ([]occupancy.Occupancy, error) {
	rows, err := s.tx.QueryContext(ctx,
		`SELECT `+occupancyColumns+` FROM resource_occupancies WHERE task_id=? ORDER BY resource_key, start_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []occupancy.Occupancy
	for rows.Next() {
		e, err := scanOccupancy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ReleaseOccupancy marks every active lease for the task released at now.
func (s *sqliteTx) ReleaseOccupancy(ctx context.Context, taskID inspection.TaskID, now int64) error {
	_, err := s.tx.ExecContext(ctx,
		`UPDATE resource_occupancies SET released_at=? WHERE task_id=? AND released_at=0`, now, taskID)
	return err
}
