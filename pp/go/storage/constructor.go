package storage

// func HeadManagerCtor(
// 	l log.Logger,
// 	clock clockwork.Clock,
// 	dataDir string,
// 	hcatalog *catalog.Catalog,
// 	blockDuration time.Duration,
// 	maxSegmentSize uint32,
// 	numberOfShards uint16,
// 	registerer prometheus.Registerer,
// ) (*HeadManager, error) {
// 	dirStat, err := os.Stat(dataDir)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to stat dir: %w", err)
// 	}

// 	if !dirStat.IsDir() {
// 		return nil, fmt.Errorf("%s is not directory", dataDir)
// 	}

// 	InitLogHandler(l)

// 	builder := NewBuilder(
// 		hcatalog,
// 		dataDir,
// 		maxSegmentSize,
// 		registerer,
// 	)

// 	loader := NewLoader(
// 		dataDir,
// 		maxSegmentSize,
// 		registerer,
// 	)

// 	h, err := uploadOrBuildHead(
// 		clock,
// 		hcatalog,
// 		builder,
// 		loader,
// 		blockDuration,
// 		numberOfShards,
// 	)
// 	if err != nil {
// 		return nil, err
// 	}

// 	if _, err = hcatalog.SetStatus(h.ID(), catalog.StatusActive); err != nil {
// 		return nil, fmt.Errorf("failed to set active status: %w", err)
// 	}

// 	activeHead := container.NewWeighted(h)

// 	m := manager.NewManager(
// 		activeHead,
// 		builder,
// 		loader,
// 		numberOfShards,
// 		registerer,
// 	)

// 	return m, nil
// }
