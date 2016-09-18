package cyclist

import "time"

var (
	standardFluxCapacitorTime, _ = time.Parse(time.RFC3339, "1955-11-05T11:05:55-09:00")
)

type lifecycleEvent struct {
	Event     string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
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
