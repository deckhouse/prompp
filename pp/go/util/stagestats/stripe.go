package stagestats

// Stage is the index of a stage in the stage names of its [Recorder].
type Stage uint8

//
// Stripe
//

// Stripe is a handle of the counters of one stripe of a [Recorder]. The zero [Stripe] accounts nothing.
type Stripe struct {
	counters []counter
}

// Enabled reports whether the [Stripe] accounts observations.
func (s Stripe) Enabled() bool {
	return s.counters != nil
}

// Observe accounts one execution of the stage with the duration d in nanoseconds.
func (s Stripe) Observe(stage Stage, d int64) {
	if s.counters == nil {
		return
	}

	c := &s.counters[stage]
	c.sum.Add(uint64(d)) // #nosec G115 // the monotonic clock duration is not negative
	c.count.Add(1)
}

// ObserveMax accounts one execution of the stage with the maximum duration of the slot over the shards,
// if any shard has set it.
func (s Stripe) ObserveMax(stage Stage, slots ShardSlots, slot int) {
	if s.counters == nil {
		return
	}

	if d := slots.Max(slot); d > 0 {
		s.Observe(stage, d)
	}
}

// Since accounts one execution of the stage that started at start, obtained from [Now].
func (s Stripe) Since(stage Stage, start int64) {
	if s.counters == nil {
		return
	}

	s.Observe(stage, Now()-start)
}
