package querier

//
//const (
//	numberOfShards = 2
//	maxSegmentSize = 1024
//
//	relabelerName = "relabeler"
//)
//
//type QuerierTestSuite struct {
//	suite.Suite
//	context context.Context
//	tmpDir  string
//	head    *head.Head
//}
//
//func (s *QuerierTestSuite) SetupTest() {
//	s.context = context.Background()
//	s.tmpDir = s.T().TempDir()
//
//	s.createHead()
//}
//
//func (s *QuerierTestSuite) createHead() {
//	var err error
//	s.head, err = head.Create(
//		"test_head_id",
//		0,
//		s.tmpDir,
//		[]*config.InputRelabelerConfig{
//			{
//				Name: relabelerName,
//			},
//		},
//		numberOfShards,
//		maxSegmentSize,
//		head.NoOpLastAppendedSegmentIDSetter{},
//		prometheus.DefaultRegisterer,
//		1,
//	)
//	s.NoError(err)
//}
//
//func (s *QuerierTestSuite) fillHead(timeSeries []headtest.TimeSeries) {
//	headtest.MustAppendTimeSeries(&s.Suite, s.head, relabelerName, timeSeries)
//}
//
//func TestQuerierTestSuite(t *testing.T) {
//	suite.Run(t, new(QuerierTestSuite))
//}
//
//func (s *QuerierTestSuite) TestRangeQuery() {
//	// Arrange
//	timeSeries := []headtest.TimeSeries{
//		{
//			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
//			Samples: []cppbridge.Sample{
//				{Timestamp: 0, Value: 1},
//			},
//		},
//		{
//			Labels: labels.FromStrings("__name__", "metric", "job", "test2"),
//			Samples: []cppbridge.Sample{
//				{Timestamp: 0, Value: 10},
//			},
//		},
//	}
//	s.fillHead(timeSeries)
//
//	q := NewQuerier(s.head, NoOpShardedDeduplicatorFactory(), 0, 2, nil, nil)
//	defer func() { _ = q.Close() }()
//	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")
//
//	// Act
//	seriesSet := q.Select(s.context, false, nil, matcher)
//
//	// Assert
//	s.Equal(timeSeries, headtest.TimeSeriesFromSeriesSet(seriesSet))
//}
//
//func (s *QuerierTestSuite) TestRangeQueryWithDataStorageLoading() {
//	// Arrange
//	timeSeries := []headtest.TimeSeries{
//		{
//			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
//			Samples: []cppbridge.Sample{
//				{Timestamp: 0, Value: 0},
//				{Timestamp: 1, Value: 1},
//				{Timestamp: 2, Value: 2},
//			},
//		},
//		{
//			Labels: labels.FromStrings("__name__", "metric", "job", "test2"),
//			Samples: []cppbridge.Sample{
//				{Timestamp: 0, Value: 10},
//				{Timestamp: 1, Value: 11},
//				{Timestamp: 2, Value: 12},
//			},
//		},
//	}
//	s.fillHead(timeSeries)
//
//	timeSeriesAfterUnload := []headtest.TimeSeries{
//		{
//			Labels: timeSeries[0].Labels,
//			Samples: []cppbridge.Sample{
//				{Timestamp: 3, Value: 3},
//			},
//		},
//		{
//			Labels: timeSeries[1].Labels,
//			Samples: []cppbridge.Sample{
//				{Timestamp: 3, Value: 13},
//			},
//		},
//	}
//
//	q := NewQuerier(s.head, NoOpShardedDeduplicatorFactory(), 0, 3, nil, nil)
//	defer func() { _ = q.Close() }()
//	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")
//
//	// Act
//	q.head.UnloadUnusedSeriesData()
//	s.fillHead(timeSeriesAfterUnload)
//	seriesSet := q.Select(s.context, false, nil, matcher)
//
//	// Assert
//	timeSeries[0].AppendSamples(timeSeriesAfterUnload[0].Samples...)
//	timeSeries[1].AppendSamples(timeSeriesAfterUnload[1].Samples...)
//	s.Equal(timeSeries, headtest.TimeSeriesFromSeriesSet(seriesSet))
//}
//
//func (s *QuerierTestSuite) TestInstantQuery() {
//	// Arrange
//	timeSeries := []headtest.TimeSeries{
//		{
//			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
//			Samples: []cppbridge.Sample{
//				{Timestamp: 0, Value: 1},
//			},
//		},
//		{
//			Labels: labels.FromStrings("__name__", "metric", "job", "test2"),
//			Samples: []cppbridge.Sample{
//				{Timestamp: 0, Value: 10},
//			},
//		},
//	}
//	s.fillHead(timeSeries)
//
//	q := NewQuerier(s.head, NoOpShardedDeduplicatorFactory(), 0, 0, nil, nil)
//	defer func() { _ = q.Close() }()
//	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")
//
//	// Act
//	seriesSet := q.Select(s.context, false, nil, matcher)
//
//	// Assert
//	s.Equal(timeSeries, headtest.TimeSeriesFromSeriesSet(seriesSet))
//}
//
//func (s *QuerierTestSuite) TestInstantQueryWithDataStorageLoading() {
//	// Arrange
//	timeSeries := []headtest.TimeSeries{
//		{
//			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
//			Samples: []cppbridge.Sample{
//				{Timestamp: 0, Value: 0},
//				{Timestamp: 1, Value: 1},
//				{Timestamp: 2, Value: 2},
//			},
//		},
//		{
//			Labels: labels.FromStrings("__name__", "metric", "job", "test2"),
//			Samples: []cppbridge.Sample{
//				{Timestamp: 0, Value: 10},
//				{Timestamp: 1, Value: 11},
//				{Timestamp: 2, Value: 12},
//			},
//		},
//	}
//	s.fillHead(timeSeries)
//
//	timeSeriesAfterUnload := []headtest.TimeSeries{
//		{
//			Labels: timeSeries[0].Labels,
//			Samples: []cppbridge.Sample{
//				{Timestamp: 3, Value: 3},
//			},
//		},
//		{
//			Labels: timeSeries[1].Labels,
//			Samples: []cppbridge.Sample{
//				{Timestamp: 3, Value: 13},
//			},
//		},
//	}
//
//	q := NewQuerier(s.head, NoOpShardedDeduplicatorFactory(), 0, 0, nil, nil)
//	defer func() { _ = q.Close() }()
//	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")
//
//	// Act
//	q.head.UnloadUnusedSeriesData()
//	s.fillHead(timeSeriesAfterUnload)
//	seriesSet := q.Select(s.context, false, nil, matcher)
//
//	// Assert
//	s.Equal([]headtest.TimeSeries{
//		{
//			Labels: timeSeries[0].Labels,
//			Samples: []cppbridge.Sample{
//				{Timestamp: 0, Value: 0},
//			},
//		},
//		{
//			Labels: timeSeries[1].Labels,
//			Samples: []cppbridge.Sample{
//				{Timestamp: 0, Value: 10},
//			},
//		},
//	}, headtest.TimeSeriesFromSeriesSet(seriesSet))
//}
