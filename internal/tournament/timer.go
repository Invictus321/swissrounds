package tournament

import "time"

// Timer is a round clock. While running, EndsAtMs (unix ms) is authoritative;
// while paused, RemainingMs is. Remaining time may go negative (overtime).
type Timer struct {
	DurationMs  int64 `json:"durationMs"`
	Running     bool  `json:"running"`
	EndsAtMs    int64 `json:"endsAtMs"`
	RemainingMs int64 `json:"remainingMs"`
}

func nowMs(now time.Time) int64 { return now.UnixMilli() }

// Remaining returns the time left on the clock at now.
func (t *Timer) Remaining(now time.Time) int64 {
	if t.Running {
		return t.EndsAtMs - nowMs(now)
	}
	return t.RemainingMs
}

func (t *Timer) Start(now time.Time) {
	if t.Running {
		return
	}
	t.Running = true
	t.EndsAtMs = nowMs(now) + t.RemainingMs
}

func (t *Timer) Pause(now time.Time) {
	if !t.Running {
		return
	}
	t.RemainingMs = t.Remaining(now)
	t.Running = false
	t.EndsAtMs = 0
}

// Reset stops the clock and restores the full round duration.
func (t *Timer) Reset() {
	t.Running = false
	t.EndsAtMs = 0
	t.RemainingMs = t.DurationMs
}

// Adjust adds (or with a negative delta, removes) time from the clock.
func (t *Timer) Adjust(deltaMs int64) {
	if t.Running {
		t.EndsAtMs += deltaMs
	} else {
		t.RemainingMs += deltaMs
	}
}

// SetDuration sets the round length and resets the clock.
func (t *Timer) SetDuration(ms int64) {
	t.DurationMs = ms
	t.Reset()
}
