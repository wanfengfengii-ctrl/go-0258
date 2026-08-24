package inspection

import (
	"sort"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
)

// EventType names the immutable audit events appended as a task advances.
type EventType string

const (
	EventTaskCreated   EventType = "task_created"
	EventSampled       EventType = "sampled"
	EventBlindSplit    EventType = "blind_split"
	EventOccupied      EventType = "occupied"
	EventColdChain     EventType = "cold_chain"
	EventReading       EventType = "reading"
	EventRejudged      EventType = "rejudged"
	EventReviewed      EventType = "reviewed"
	EventFinalized     EventType = "finalized"
	EventIdempotentHit EventType = "idempotent_hit"
)

// AuditEvent is an append-only, generation-stamped record of one meaningful
// transition or rejection. Audit events are never mutated and survive restarts
// so the inspection trail is fully replayable.
type AuditEvent struct {
	Sequence    int64            `json:"sequence"`
	TaskID      TaskID           `json:"taskId"`
	Generation  Generation       `json:"generation"`
	EventType   EventType        `json:"eventType"`
	Actor       catalog.PersonID `json:"actor,omitempty"`
	Detail      string           `json:"detail,omitempty"`
	LogicalTime int64            `json:"logicalTime"`
}

// AuditLog is an ordered collection of audit events for a task.
type AuditLog struct {
	events []AuditEvent
	seq    int64
}

// Append records a new audit event with the next sequence number and logical
// time. It returns the recorded event.
func (l *AuditLog) Append(task TaskID, generation Generation, typ EventType, actor catalog.PersonID, detail string, logicalTime int64) AuditEvent {
	l.seq++
	ev := AuditEvent{
		Sequence:    l.seq,
		TaskID:      task,
		Generation:  generation,
		EventType:   typ,
		Actor:       actor,
		Detail:      detail,
		LogicalTime: logicalTime,
	}
	l.events = append(l.events, ev)
	return ev
}

// Events returns the events in append order.
func (l *AuditLog) Events() []AuditEvent {
	out := make([]AuditEvent, len(l.events))
	copy(out, l.events)
	return out
}

// RebuildAuditLog reconstructs an audit log from persisted events, restoring
// the sequence counter so subsequent appends continue monotonically.
func RebuildAuditLog(events []AuditEvent) *AuditLog {
	sort.SliceStable(events, func(i, j int) bool { return events[i].Sequence < events[j].Sequence })
	l := &AuditLog{events: events}
	for _, e := range events {
		if e.Sequence > l.seq {
			l.seq = e.Sequence
		}
	}
	return l
}
