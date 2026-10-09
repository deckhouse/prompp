package stagestats

//
// Lap
//

// Lap accounts consecutive stages: each [Lap.Mark] accounts the time since the previous mark (or the start)
// to the stage, so N stages take N+1 clock readings and their sum is the total time without gaps.
type Lap struct {
	stripe Stripe
	start  int64
	prev   int64
}

// Enabled reports whether the [Lap] accounts stages. A nil [Lap] is disabled.
func (l *Lap) Enabled() bool {
	return l != nil && l.stripe.Enabled()
}

// Mark accounts the time since the previous mark to the stage. A nil or disabled [Lap] does nothing.
func (l *Lap) Mark(stage Stage) {
	if !l.Enabled() {
		return
	}

	now := Now()
	l.stripe.Observe(stage, now-l.prev)
	l.prev = now
}

// SinceMicroseconds returns the time elapsed since the start of the [Lap].
func (l *Lap) SinceMicroseconds() float64 {
	return float64(Now()-l.start) / 1e3
}
