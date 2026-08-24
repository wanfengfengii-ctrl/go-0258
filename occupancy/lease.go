package occupancy

// Lease validation and conflict-detection helpers shared by the plate-well and
// incubator ledgers. A lease is only valid when its interval is non-empty,
// its resource identity matches its type, and its task and generation are
// present.

// Validate reports the first validity problem with a lease.
func (o *Occupancy) Validate() error {
	if o.TaskID == "" {
		return ErrInvalidLease
	}
	if o.StartAt < 0 || o.EndAt <= o.StartAt {
		return ErrInvalidLease
	}
	switch o.ResourceType {
	case ResourcePlateWell:
		if o.PlateID == "" || o.Well == "" {
			return ErrInvalidLease
		}
	case ResourceIncubator:
		if o.IncubatorID == "" {
			return ErrInvalidLease
		}
	default:
		return ErrInvalidLease
	}
	return nil
}

// Overlaps reports whether two leases overlap in time on the same resource.
// It is the time-interval half-open test [StartAt, EndAt).
func (o *Occupancy) Overlaps(other *Occupancy) bool {
	if o.ResourceType != other.ResourceType {
		return false
	}
	if !o.sameResource(other) {
		return false
	}
	return o.StartAt < other.EndAt && other.StartAt < o.EndAt
}

func (o *Occupancy) sameResource(other *Occupancy) bool {
	switch o.ResourceType {
	case ResourcePlateWell:
		return o.PlateID == other.PlateID && o.Well == other.Well
	case ResourceIncubator:
		return o.IncubatorID == other.IncubatorID
	default:
		return false
	}
}

// ResourceKey returns a stable identity string for the resource held by the
// lease, used as a uniqueness key by the persistence layer.
func (o *Occupancy) ResourceKey() string {
	switch o.ResourceType {
	case ResourcePlateWell:
		return string(ResourcePlateWell) + ":" + o.PlateID + ":" + o.Well
	case ResourceIncubator:
		return string(ResourceIncubator) + ":" + o.IncubatorID
	default:
		return ""
	}
}

// Active reports whether the lease is currently held (not released).
func (o *Occupancy) Active() bool { return o.ReleasedAt == 0 }

// IsConflictResult reports whether err is an occupancy conflict.
func IsConflictResult(err error) bool { return err == ErrOccupied }
