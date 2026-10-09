package stagestats_test

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/util/stagestats"
)

const (
	stageFirst stagestats.Stage = iota
	stageSecond
	stageThird
)

var stageNames = []string{"first", "second", "third"}

type RecorderSuite struct {
	suite.Suite

	registry *prometheus.Registry
	recorder *stagestats.Recorder
}

func TestRecorderSuite(t *testing.T) {
	suite.Run(t, new(RecorderSuite))
}

func (s *RecorderSuite) SetupTest() {
	s.registry = prometheus.NewRegistry()
	s.recorder = stagestats.NewRecorder(
		s.registry,
		stagestats.Opts{Name: "test_stage", Help: "Test pipeline.", ConstLabels: prometheus.Labels{"source": "test"}},
		stageNames,
	)
}

func (s *RecorderSuite) TestNowIsMonotonic() {
	// Arrange
	first := stagestats.Now()

	// Act
	second := stagestats.Now()

	// Assert
	s.LessOrEqual(first, second)
}

func (s *RecorderSuite) TestObserveSumsOverStripes() {
	// Arrange
	first := s.recorder.Stripe(0)
	second := s.recorder.Stripe(1)

	// Act
	first.Observe(stageFirst, 10)
	second.Observe(stageFirst, 20)
	second.Observe(stageThird, 5)

	// Assert
	s.Require().NoError(testutil.CollectAndCompare(s.recorder, strings.NewReader(`
# HELP test_stage_duration_nanoseconds_total Test pipeline. Total duration of the stage in nanoseconds.
# TYPE test_stage_duration_nanoseconds_total counter
test_stage_duration_nanoseconds_total{source="test",stage="first"} 30
test_stage_duration_nanoseconds_total{source="test",stage="second"} 0
test_stage_duration_nanoseconds_total{source="test",stage="third"} 5
# HELP test_stage_executions_total Test pipeline. Total number of the stage executions.
# TYPE test_stage_executions_total counter
test_stage_executions_total{source="test",stage="first"} 2
test_stage_executions_total{source="test",stage="second"} 0
test_stage_executions_total{source="test",stage="third"} 1
`)))
}

func (s *RecorderSuite) TestNewRecorderReturnsRegistered() {
	// Act
	recorder := stagestats.NewRecorder(
		s.registry,
		stagestats.Opts{Name: "test_stage", Help: "Test pipeline.", ConstLabels: prometheus.Labels{"source": "test"}},
		stageNames,
	)

	// Assert
	s.Same(s.recorder, recorder)
}

func (s *RecorderSuite) TestLapMarksConsecutiveStages() {
	// Arrange
	start := stagestats.Now()
	lap := s.recorder.Start()

	// Act
	lap.Mark(stageFirst)
	lap.Mark(stageSecond)
	lap.Mark(stageThird)
	total := stagestats.Now() - start

	// Assert
	s.Require().NoError(testutil.CollectAndCompare(s.recorder, strings.NewReader(`
# HELP test_stage_executions_total Test pipeline. Total number of the stage executions.
# TYPE test_stage_executions_total counter
test_stage_executions_total{source="test",stage="first"} 1
test_stage_executions_total{source="test",stage="second"} 1
test_stage_executions_total{source="test",stage="third"} 1
`), "test_stage_executions_total"))
	s.Positive(s.durationSum())
	s.LessOrEqual(s.durationSum(), float64(total))
	s.LessOrEqual(lap.SinceMicroseconds(), float64(stagestats.Now()-start)/1e3)
}

func (s *RecorderSuite) TestNilRecorderAccountsNothing() {
	// Arrange
	var recorder *stagestats.Recorder
	lap := recorder.Start()
	stripe := recorder.Stripe(0)
	slots := stagestats.NewShardSlots(1)
	slots.Set(0, 0, 10)

	// Act
	lap.Mark(stageFirst)
	stripe.Observe(stageFirst, 10)
	stripe.Since(stageFirst, stagestats.Now())
	stripe.ObserveMax(stageFirst, slots, 0)

	// Assert
	s.False(lap.Enabled())
	s.False(stripe.Enabled())
}

func (s *RecorderSuite) TestNilLapIsDisabled() {
	// Arrange
	var lap *stagestats.Lap

	// Act
	lap.Mark(stageFirst)

	// Assert
	s.False(lap.Enabled())
}

func (s *RecorderSuite) TestObserveMaxOverShards() {
	// Arrange
	slots := stagestats.NewShardSlots(3)
	slots.Set(0, 0, 10)
	slots.Set(1, 0, 40)
	slots.Set(2, 0, 12)
	stripe := s.recorder.Stripe(0)

	// Act
	stripe.ObserveMax(stageFirst, slots, 0)
	stripe.ObserveMax(stageSecond, slots, 1)

	// Assert
	s.Require().NoError(testutil.CollectAndCompare(s.recorder, strings.NewReader(`
# HELP test_stage_executions_total Test pipeline. Total number of the stage executions.
# TYPE test_stage_executions_total counter
test_stage_executions_total{source="test",stage="first"} 1
test_stage_executions_total{source="test",stage="second"} 0
test_stage_executions_total{source="test",stage="third"} 0
# HELP test_stage_duration_nanoseconds_total Test pipeline. Total duration of the stage in nanoseconds.
# TYPE test_stage_duration_nanoseconds_total counter
test_stage_duration_nanoseconds_total{source="test",stage="first"} 40
test_stage_duration_nanoseconds_total{source="test",stage="second"} 0
test_stage_duration_nanoseconds_total{source="test",stage="third"} 0
`)))
}

func (s *RecorderSuite) TestShardSlotsReset() {
	// Arrange
	slots := stagestats.NewShardSlots(2)
	slots.Set(1, 3, 10)

	// Act
	slots.Reset()

	// Assert
	s.Zero(slots.Max(3))
}

func (s *RecorderSuite) TestShardSlotsSinceConsecutiveSlots() {
	// Arrange
	slots := stagestats.NewShardSlots(2)
	start := slots.Start()

	// Act
	next := slots.Since(1, 0, start)
	end := slots.Since(1, 1, next)

	// Assert
	s.Equal(next-start, slots.Max(0))
	s.Equal(end-next, slots.Max(1))
}

func (s *RecorderSuite) TestNilShardSlotsDoNotReadClock() {
	// Arrange
	var slots stagestats.ShardSlots

	// Act
	start := slots.Start()
	next := slots.Since(0, 0, start)

	// Assert
	s.Zero(start)
	s.Zero(next)
}

// durationSum returns the total duration over all stages.
func (s *RecorderSuite) durationSum() float64 {
	families, err := s.registry.Gather()
	s.Require().NoError(err)

	var total float64
	for _, family := range families {
		if family.GetName() != "test_stage_duration_nanoseconds_total" {
			continue
		}

		for _, metric := range family.GetMetric() {
			total += metric.GetCounter().GetValue()
		}
	}

	return total
}
