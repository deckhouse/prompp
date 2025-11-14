package storage

// NewMergeQuerierConcurrent returns a new Querier that merges results of given primary and secondary queriers.
// Run concurrent select if there are multiple Queriers.
func NewMergeQuerierConcurrent(primaries, secondaries []Querier, mergeFn VerticalSeriesMergeFunc) Querier {
	primaries = filterQueriers(primaries)
	secondaries = filterQueriers(secondaries)

	switch {
	case len(primaries) == 0 && len(secondaries) == 0:
		return noopQuerier{}
	case len(primaries) == 1 && len(secondaries) == 0:
		return primaries[0]
	case len(primaries) == 0 && len(secondaries) == 1:
		return &querierAdapter{newSecondaryQuerierFrom(secondaries[0])}
	}

	queriers := make([]genericQuerier, 0, len(primaries)+len(secondaries))
	for _, q := range primaries {
		queriers = append(queriers, newGenericQuerierFrom(q))
	}
	for _, q := range secondaries {
		queriers = append(queriers, newSecondaryQuerierFrom(q))
	}

	concurrentSelect := len(secondaries) > 0 || len(primaries) > 1

	return &querierAdapter{&mergeGenericQuerier{
		mergeFn:          (&seriesMergerAdapter{VerticalSeriesMergeFunc: mergeFn}).Merge,
		queriers:         queriers,
		concurrentSelect: concurrentSelect,
	}}
}
