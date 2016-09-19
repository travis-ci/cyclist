package cyclist

import (
	"fmt"
	"time"
)

var (
	standardFluxCapacitorTime, _ = time.Parse(time.RFC3339, "1955-11-05T11:05:55-09:00")
)

type lifecycleEvent struct {
	Event     string
	Timestamp time.Time
}

func newLifecycleEvent(event, ts string) *lifecycleEvent {
	timestamp, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		timestamp = standardFluxCapacitorTime
	}

	return &lifecycleEvent{
		Event:     event,
		Timestamp: timestamp,
	}
}

func (le *lifecycleEvent) MarshalJSON() ([]byte, error) {
	if le.Timestamp == standardFluxCapacitorTime || le.Timestamp.IsZero() {
		return []byte(fmt.Sprintf(`{"event":%q,"timestamp":null}`, le.Event)), nil
	}

	return []byte(fmt.Sprintf(`{"event":%q,"timestamp":%q}`,
		le.Event, le.Timestamp.Format(time.RFC3339Nano))), nil
}
