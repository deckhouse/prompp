package cppbridge_test

import (
	"context"
	"fmt"
	"testing"
	"time"
	"unsafe"

	"github.com/cespare/xxhash/v2"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
)

type RelabelerSuite struct {
	suite.Suite
	baseCtx context.Context
	options cppbridge.RelabelerOptions
}

func TestRelabelerSuite(t *testing.T) {
	suite.Run(t, new(RelabelerSuite))
}

func (s *RelabelerSuite) SetupTest() {
	s.baseCtx = context.Background()
}

func (s *RelabelerSuite) TestRelabelConfigValidate() {
	tests := []struct {
		expected string
		config   cppbridge.RelabelConfig
	}{
		{
			config:   cppbridge.RelabelConfig{},
			expected: `relabel action cannot be empty`,
		},
		{
			config: cppbridge.RelabelConfig{
				Action: cppbridge.Replace,
			},
			expected: `requires 'target_label' value`,
		},
		{
			config: cppbridge.RelabelConfig{
				Action: cppbridge.Lowercase,
			},
			expected: `requires 'target_label' value`,
		},
		{
			config: cppbridge.RelabelConfig{
				Action:      cppbridge.Lowercase,
				Replacement: "$1",
				TargetLabel: "${3}",
			},
			expected: `"${3}" is invalid 'target_label'`,
		},
		{
			config: cppbridge.RelabelConfig{
				SourceLabels: []string{"a"},
				Regex:        "some-([^-]+)-([^,]+)",
				Action:       cppbridge.Replace,
				Replacement:  "${1}",
				TargetLabel:  "${3}",
			},
		},
		{
			config: cppbridge.RelabelConfig{
				SourceLabels: []string{"a"},
				Regex:        "some-([^-]+)-([^,]+)",
				Action:       cppbridge.Replace,
				Replacement:  "${1}",
				TargetLabel:  "0${3}",
			},
			expected: `"0${3}" is invalid 'target_label'`,
		},
		{
			config: cppbridge.RelabelConfig{
				SourceLabels: []string{"a"},
				Regex:        "some-([^-]+)-([^,]+)",
				Action:       cppbridge.Replace,
				Replacement:  "${1}",
				TargetLabel:  "-${3}",
			},
			expected: `"-${3}" is invalid 'target_label' for replace action`,
		},
	}
	for i, test := range tests {
		s.Run(fmt.Sprint(i), func() {
			err := test.config.Validate()
			if test.expected == "" {
				s.Require().NoError(err)
			} else {
				s.Require().ErrorContains(err, test.expected)
			}
		})
	}
}

func (s *RelabelerSuite) TestTargetLabelValidity() {
	tests := []struct {
		str   string
		valid bool
	}{
		{"-label", false},
		{"label", true},
		{"label${1}", true},
		{"${1}label", true},
		{"${1}", true},
		{"${1}label", true},
		{"${", false},
		{"$", false},
		{"${}", false},
		{"foo${", false},
		{"$1", true},
		{"asd$2asd", true},
		{"-foo${1}bar-", false},
		{"_${1}_", true},
		{"foo${bar}foo", true},
	}
	for _, test := range tests {
		s.Require().Equal(test.valid, cppbridge.RelabelTarget.MatchString(test.str),
			"Expected %q to be %v", test.str, test.valid)
	}
}

func (s *RelabelerSuite) TestAction() {
	raw := `action: Labelkeep`

	c := struct {
		Action cppbridge.Action `yaml:"action"`
	}{}

	err := yaml.Unmarshal([]byte(raw), &c)
	s.Require().NoError(err)

	s.Require().Equal(cppbridge.LabelKeep, c.Action)
}

func (s *RelabelerSuite) TestOutputPerShardRelabeler() {
	rCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"job"},
			Regex:        "abc",
			Action:       cppbridge.Keep,
		},
	}

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(rCfgs)
	s.Require().NoError(err)

	externalLabels := []cppbridge.Label{
		{"name0", "value0"},
		{"name1", "value1"},
	}

	_, err = cppbridge.NewOutputPerShardRelabeler(externalLabels, statelessRelabeler, 0, 0, 0)
	s.Require().NoError(err)
}

func (s *RelabelerSuite) TestCacheAllocatedMemory() {
	cache := cppbridge.NewCache()
	s.Equal(uint64(16), cache.AllocatedMemory())
}

func (s *RelabelerSuite) TestToHash_EmptySlice() {
	rCfgs := []*cppbridge.RelabelConfig{}

	s.Require().Equal(xxhash.Sum64String(""), cppbridge.ToHash(rCfgs))
}

func (s *RelabelerSuite) TestToHash_EmptyConfig() {
	rCfgs := []*cppbridge.RelabelConfig{{}}
	var a cppbridge.Action

	s.Require().Equal(xxhash.Sum64String("0"+a.String()), cppbridge.ToHash(rCfgs))
}

//
// PerGoroutineRelabelerSuite
//

type PerGoroutineRelabelerSuite struct {
	suite.Suite
	baseCtx        context.Context
	options        cppbridge.RelabelerOptions
	hlimits        cppbridge.WALHashdexLimits
	rCfgs          []*cppbridge.RelabelConfig
	inputLss       *cppbridge.LabelSetStorage
	targetLss      *cppbridge.LabelSetStorage
	numberOfShards uint16
}

func TestPerGoroutineRelabelerSuite(t *testing.T) {
	suite.Run(t, new(PerGoroutineRelabelerSuite))
}

func (s *PerGoroutineRelabelerSuite) SetupSuite() {
	s.baseCtx = context.Background()
	s.hlimits = cppbridge.DefaultWALHashdexLimits()
	s.rCfgs = []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"job"},
			Regex:        "abc",
			Action:       cppbridge.Keep,
		},
	}
	s.numberOfShards = 1
}

func (s *PerGoroutineRelabelerSuite) SetupTest() {
	s.options = cppbridge.RelabelerOptions{}
	s.inputLss = cppbridge.NewLssStorage()
	s.targetLss = cppbridge.NewQueryableLssStorage()
}

func (s *PerGoroutineRelabelerSuite) TestInvalidAction() {
	h, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	})
	s.Require().NoError(err)

	rCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"job"},
			Regex:        "abc",
			Action:       cppbridge.Action(20),
		},
	}

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(s.numberOfShards)

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(rCfgs)
	s.Require().NoError(err)

	state := cppbridge.NewStateV2WithoutLock()
	state.SetRelabelerOptions(&s.options)
	state.SetStatelessRelabeler(statelessRelabeler)
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, hasReallocations, err := pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().Error(err)
	s.Equal(cppbridge.RelabelerStats{}, stats)
	s.False(hasReallocations)
}

func (s *PerGoroutineRelabelerSuite) TestRelabeling() {
	h, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	})
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(s.numberOfShards)

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(s.rCfgs)
	s.Require().NoError(err)

	state := cppbridge.NewStateV2WithoutLock()
	state.SetRelabelerOptions(&s.options)
	state.SetStatelessRelabeler(statelessRelabeler)
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, hasReallocations, err := pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 1, 1}, stats)
	s.True(hasReallocations)
	s.Equal(uint64(1), shardsInnerSeries[0].Size())
}

func (s *PerGoroutineRelabelerSuite) TestRelabelingDrop() {
	h, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	})
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(s.numberOfShards)

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(s.rCfgs)
	s.Require().NoError(err)

	state := cppbridge.NewStateV2WithoutLock()
	state.SetRelabelerOptions(&s.options)
	state.SetStatelessRelabeler(statelessRelabeler)
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, hasReallocations, err := pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{0, 0, 2}, stats)
	s.True(hasReallocations)
}

func (s *PerGoroutineRelabelerSuite) TestRelabelingFromCacheTrue() {
	h, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	})
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(s.numberOfShards)

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(s.rCfgs)
	s.Require().NoError(err)

	state := cppbridge.NewStateV2WithoutLock()
	state.SetStatelessRelabeler(statelessRelabeler)
	state.SetRelabelerOptions(&s.options)
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, hasReallocations, err := pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 1, 0}, stats)
	s.True(hasReallocations)

	stats, ok, err := pgr.RelabelingFromCache(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 0, 0}, stats)
	s.True(ok)
}

func (s *PerGoroutineRelabelerSuite) TestRelabelingFromCacheFalse() {
	h, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	})
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	state := cppbridge.NewStateV2WithoutLock()
	state.SetRelabelerOptions(&s.options)
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, ok, err := pgr.RelabelingFromCache(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{0, 0, 0}, stats)
	s.False(ok)
}

func (s *PerGoroutineRelabelerSuite) TestRelabelingFromCachePartially() {
	ts := time.Now().UnixMilli()
	h1, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value0"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value0"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value1"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value2"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value2"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value3"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value3"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value3"},
					{Name: "instance", Value: "value3"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value4"},
					{Name: "instance", Value: "value4"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value5"},
					{Name: "instance", Value: "value5"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value6"},
					{Name: "instance", Value: "value6"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
		},
	})
	s.Require().NoError(err)

	ts += 6000
	h2, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value0"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value0"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value1"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value2"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value2"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value3"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value3"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value4"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value4"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
		},
	})
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(s.numberOfShards)

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(s.rCfgs)
	s.Require().NoError(err)

	state := cppbridge.NewStateV2WithoutLock()
	state.SetRelabelerOptions(&s.options)
	state.SetStatelessRelabeler(statelessRelabeler)
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, hasReallocations, err := pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h1,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{4, 4, 4}, stats)
	s.True(hasReallocations)

	shardsInnerSeries = cppbridge.NewShardsInnerSeries(s.numberOfShards)
	stats, ok, err := pgr.RelabelingFromCache(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h2,
		shardsInnerSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{4, 0, 0}, stats)
	s.False(ok)
	s.Equal(uint64(4), shardsInnerSeries[0].Size())

	stats, _, err = pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h2,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 1, 0}, stats)
	s.Equal(uint64(5), shardsInnerSeries[0].Size())
}

func (s *PerGoroutineRelabelerSuite) TestRelabelingTransition() {
	h, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	})
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(s.numberOfShards)

	state := cppbridge.NewTransitionStateV2()
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, hasReallocations, err := pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{2, 2, 0}, stats)
	s.True(hasReallocations)
}

func (s *PerGoroutineRelabelerSuite) TestRelabelingFromCacheTrueTransition() {
	h, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	})
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(s.numberOfShards)

	state := cppbridge.NewTransitionStateV2()
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, hasReallocations, err := pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 1, 0}, stats)
	s.True(hasReallocations)

	stats, ok, err := pgr.RelabelingFromCache(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 0, 0}, stats)
	s.True(ok)
}

func (s *PerGoroutineRelabelerSuite) TestRelabelingFromCacheFalseTransition() {
	h, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	})
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	state := cppbridge.NewTransitionStateV2()
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, ok, err := pgr.RelabelingFromCache(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{0, 0, 0}, stats)
	s.False(ok)
}

func (s *PerGoroutineRelabelerSuite) TestRelabelingFromCachePartiallyTransition() {
	ts := time.Now().UnixMilli()
	h1, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value0"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value0"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value1"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value2"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value2"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value3"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value3"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value3"},
					{Name: "instance", Value: "value3"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value4"},
					{Name: "instance", Value: "value4"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value5"},
					{Name: "instance", Value: "value5"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value6"},
					{Name: "instance", Value: "value6"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
		},
	})
	s.Require().NoError(err)

	ts += 6000
	h2, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value0"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value0"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value1"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value2"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value2"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value3"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value3"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value4"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value4"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
		},
	})
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(s.numberOfShards)

	state := cppbridge.NewTransitionStateV2()
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, hasReallocations, err := pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h1,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{8, 8, 0}, stats)
	s.True(hasReallocations)

	shardsInnerSeries = cppbridge.NewShardsInnerSeries(s.numberOfShards)
	stats, ok, err := pgr.RelabelingFromCache(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h2,
		shardsInnerSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{4, 0, 0}, stats)
	s.False(ok)
	s.Equal(uint64(4), shardsInnerSeries[0].Size())

	stats, _, err = pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h2,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 1, 0}, stats)
	s.Equal(uint64(5), shardsInnerSeries[0].Size())
}

func (s *PerGoroutineRelabelerSuite) TestRelabelingWithStalenans() {
	h, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	})
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(s.numberOfShards)

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(s.rCfgs)
	s.Require().NoError(err)

	state := cppbridge.NewStateV2WithoutLock()
	state.SetRelabelerOptions(&s.options)
	state.SetStatelessRelabeler(statelessRelabeler)
	state.EnableTrackStaleness()
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, hasReallocations, err := pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	cppbridge.PerGoroutineRelabelerTrackStaleNans(shardsInnerSeries, state, 0)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 1, 1}, stats)
	s.True(hasReallocations)
	s.Equal(uint64(1), shardsInnerSeries[0].Size())

	h, err = s.makeSnappyProtobufHashdex(&prompb.WriteRequest{})
	s.Require().NoError(err)

	shardsInnerSeries = cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries = cppbridge.NewShardsRelabeledSeries(s.numberOfShards)
	stats, hasReallocations, err = pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{0, 0, 0}, stats)
	s.False(hasReallocations)

	cppbridge.PerGoroutineRelabelerTrackStaleNans(shardsInnerSeries, state, 0)
	s.Equal(uint64(1), shardsInnerSeries[0].Size())
}

func (s *PerGoroutineRelabelerSuite) TestRelabelingWithStalenansFromCacheTrue() {
	h, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	})
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(s.numberOfShards)

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(s.rCfgs)
	s.Require().NoError(err)

	state := cppbridge.NewStateV2WithoutLock()
	state.SetStatelessRelabeler(statelessRelabeler)
	state.SetRelabelerOptions(&s.options)
	state.EnableTrackStaleness()
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, hasReallocations, err := pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 1, 0}, stats)
	s.True(hasReallocations)

	stats, ok, err := pgr.RelabelingFromCache(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
	)
	cppbridge.PerGoroutineRelabelerTrackStaleNans(shardsInnerSeries, state, 0)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 0, 0}, stats)
	s.True(ok)

	h, err = s.makeSnappyProtobufHashdex(&prompb.WriteRequest{})
	s.Require().NoError(err)

	shardsInnerSeries = cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries = cppbridge.NewShardsRelabeledSeries(s.numberOfShards)
	stats, hasReallocations, err = pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	cppbridge.PerGoroutineRelabelerTrackStaleNans(shardsInnerSeries, state, 0)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{0, 0, 0}, stats)
	s.False(hasReallocations)
	s.Equal(uint64(1), shardsInnerSeries[0].Size())
}

func (s *PerGoroutineRelabelerSuite) TestRelabelingWithStalenansFromCacheFalse() {
	h, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	})
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	state := cppbridge.NewStateV2WithoutLock()
	state.SetRelabelerOptions(&s.options)
	state.EnableTrackStaleness()
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, ok, err := pgr.RelabelingFromCache(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
	)
	cppbridge.PerGoroutineRelabelerTrackStaleNans(shardsInnerSeries, state, 0)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{0, 0, 0}, stats)
	s.False(ok)
}

func (s *PerGoroutineRelabelerSuite) TestRelabelingWithStalenansFromCachePartially() {
	ts := time.Now().UnixMilli()
	h1, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value0"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value0"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value1"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value2"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value2"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value3"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value3"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value3"},
					{Name: "instance", Value: "value3"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value4"},
					{Name: "instance", Value: "value4"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value5"},
					{Name: "instance", Value: "value5"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value6"},
					{Name: "instance", Value: "value6"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
		},
	})
	s.Require().NoError(err)

	ts += 6000
	h2, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value0"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value0"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value1"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value2"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value2"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value3"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value3"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value4"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value4"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
		},
	})
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(s.numberOfShards)

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(s.rCfgs)
	s.Require().NoError(err)

	state := cppbridge.NewStateV2WithoutLock()
	state.SetRelabelerOptions(&s.options)
	state.SetStatelessRelabeler(statelessRelabeler)
	state.EnableTrackStaleness()
	state.Reconfigure(0, s.numberOfShards, make([]*cppbridge.IdsMapping, s.numberOfShards))

	pgr := cppbridge.NewPerGoroutineRelabeler(s.numberOfShards, 0)
	stats, hasReallocations, err := pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h1,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	cppbridge.PerGoroutineRelabelerTrackStaleNans(shardsInnerSeries, state, 0)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{4, 4, 4}, stats)
	s.True(hasReallocations)

	shardsInnerSeries = cppbridge.NewShardsInnerSeries(s.numberOfShards)
	stats, ok, err := pgr.RelabelingFromCache(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h2,
		shardsInnerSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{4, 0, 0}, stats)
	s.False(ok)
	s.Equal(uint64(4), shardsInnerSeries[0].Size())

	stats, _, err = pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h2,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	cppbridge.PerGoroutineRelabelerTrackStaleNans(shardsInnerSeries, state, 0)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 1, 0}, stats)
	s.Equal(uint64(5), shardsInnerSeries[0].Size())

	h, err := s.makeSnappyProtobufHashdex(&prompb.WriteRequest{})
	s.Require().NoError(err)

	shardsInnerSeries = cppbridge.NewShardsInnerSeries(s.numberOfShards)
	shardsRelabeledSeries = cppbridge.NewShardsRelabeledSeries(s.numberOfShards)
	stats, hasReallocations, err = pgr.Relabeling(
		s.baseCtx,
		s.inputLss,
		s.targetLss,
		state,
		h,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	cppbridge.PerGoroutineRelabelerTrackStaleNans(shardsInnerSeries, state, 0)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{0, 0, 0}, stats)
	s.False(hasReallocations)
	s.Equal(uint64(5), shardsInnerSeries[0].Size())
}

func (s *PerGoroutineRelabelerSuite) makeSnappyProtobufHashdex(
	wr *prompb.WriteRequest,
) (cppbridge.ShardedData, error) {
	data, err := wr.Marshal()
	if err != nil {
		return nil, err
	}

	return cppbridge.NewWALSnappyProtobufHashdex(snappy.Encode(nil, data), s.hlimits)
}

//
// StateV2Suite
//

type StateV2Suite struct {
	suite.Suite
}

func TestStateV2Suite(t *testing.T) {
	suite.Run(t, new(StateV2Suite))
}

func (s *StateV2Suite) TestInitState() {
	s.initState(cppbridge.NewStateV2())
	s.initState(cppbridge.NewStateV2WithoutLock())
}

func (s *StateV2Suite) initState(state *cppbridge.StateV2) {
	s.Panics(func() { state.CacheByShard(0) })
	s.GreaterOrEqual(state.DefTimestamp(), time.Now().UnixMilli())

	newDeftime := time.Now().Add(5 * time.Minute).UnixMilli()
	state.SetDefTimestamp(newDeftime)
	s.Equal(newDeftime, state.DefTimestamp())

	s.False(state.IsTransition())
	s.Equal(cppbridge.RelabelerOptions{}, state.RelabelerOptions())
	s.Panics(func() { state.StaleNansStateByShard(0) })
	s.False(state.TrackStaleness())
}

func (s *StateV2Suite) TestStateReconfigure() {
	s.stateReconfigure(cppbridge.NewStateV2())
	s.stateReconfigure(cppbridge.NewStateV2WithoutLock())
}

func (s *StateV2Suite) stateReconfigure(state *cppbridge.StateV2) {
	state.Reconfigure(0, 1, nil)

	s.NotNil(state.CacheByShard(0))
	s.False(state.TrackStaleness())
	s.Panics(func() { state.StaleNansStateByShard(0) })
}

func (s *StateV2Suite) TestStateReconfigureWithoutReconfigure() {
	s.stateReconfigureWithoutReconfigure(cppbridge.NewStateV2())
	s.stateReconfigureWithoutReconfigure(cppbridge.NewStateV2WithoutLock())
}

func (s *StateV2Suite) stateReconfigureWithoutReconfigure(state *cppbridge.StateV2) {
	state.Reconfigure(0, 1, nil)

	cache1 := state.CacheByShard(0)
	s.NotNil(cache1)

	state.Reconfigure(0, 1, nil)
	cache2 := state.CacheByShard(0)
	s.NotNil(cache2)
	s.Equal(cache1, cache2)
}

func (s *StateV2Suite) TestStateReconfigureNumberOfShards() {
	s.stateReconfigureNumberOfShards(cppbridge.NewStateV2())
	s.stateReconfigureNumberOfShards(cppbridge.NewStateV2WithoutLock())
}

func (s *StateV2Suite) stateReconfigureNumberOfShards(state *cppbridge.StateV2) {
	state.EnableTrackStaleness()
	state.Reconfigure(0, 2, make([]*cppbridge.IdsMapping, 2))

	cache0 := state.CacheByShard(0)
	s.NotNil(cache0)
	cache1 := state.CacheByShard(1)
	s.NotNil(cache1)

	state.Reconfigure(1, 1, make([]*cppbridge.IdsMapping, 1))
	newCache0 := state.CacheByShard(0)
	s.NotNil(newCache0)
	s.NotEqual(cache0, newCache0)
	s.Panics(func() { state.CacheByShard(1) })
}

func (s *StateV2Suite) TestStateReconfigureTrackStaleness() {
	s.stateReconfigureTrackStaleness(cppbridge.NewStateV2())
	s.stateReconfigureTrackStaleness(cppbridge.NewStateV2WithoutLock())
}

func (s *StateV2Suite) stateReconfigureTrackStaleness(state *cppbridge.StateV2) {
	state.EnableTrackStaleness()
	state.Reconfigure(0, 1, make([]*cppbridge.IdsMapping, 1))

	s.NotNil(state.CacheByShard(0))
	s.True(state.TrackStaleness())
	s.NotNil(state.StaleNansStateByShard(0))
}

func (s *StateV2Suite) TestStatelessRelabeler() {
	s.statelessRelabeler(cppbridge.NewStateV2())
	s.statelessRelabeler(cppbridge.NewStateV2WithoutLock())
}

func (s *StateV2Suite) statelessRelabeler(state *cppbridge.StateV2) {
	s.Panics(func() { state.StatelessRelabeler() })

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler([]*cppbridge.RelabelConfig{})
	s.Require().NoError(err)

	state.SetStatelessRelabeler(statelessRelabeler)
	s.NotNil(state.StatelessRelabeler())
}

func (s *StateV2Suite) TestInitTransitionStateV2() {
	s.initTransitionState(cppbridge.NewTransitionStateV2())
	s.initTransitionState(cppbridge.NewTransitionStateV2WithoutLock())
}

func (s *StateV2Suite) initTransitionState(state *cppbridge.StateV2) {
	s.True(state.IsTransition())
	s.Equal(cppbridge.RelabelerOptions{}, state.RelabelerOptions())
	s.Panics(func() { state.CacheByShard(0) })
	s.Panics(func() { state.StaleNansStateByShard(0) })
	s.False(state.TrackStaleness())
}

func (s *StateV2Suite) TestStateTransitionReconfigure() {
	s.stateTransitionReconfigure(cppbridge.NewTransitionStateV2())
	s.stateTransitionReconfigure(cppbridge.NewTransitionStateV2WithoutLock())
}

func (s *StateV2Suite) stateTransitionReconfigure(state *cppbridge.StateV2) {
	state.Reconfigure(0, 1, make([]*cppbridge.IdsMapping, 1))

	s.False(state.TrackStaleness())
	s.Panics(func() { state.CacheByShard(0) })
	s.Panics(func() { state.StaleNansStateByShard(0) })
}

func (s *StateV2Suite) TestStateTransitionReconfigureTrackStaleness() {
	s.stateTransitionReconfigureTrackStaleness(cppbridge.NewTransitionStateV2())
	s.stateTransitionReconfigureTrackStaleness(cppbridge.NewTransitionStateV2WithoutLock())
}

func (s *StateV2Suite) stateTransitionReconfigureTrackStaleness(state *cppbridge.StateV2) {
	s.Panics(func() { state.EnableTrackStaleness() })
	state.Reconfigure(0, 1, make([]*cppbridge.IdsMapping, 1))

	s.False(state.TrackStaleness())
	s.Panics(func() { state.CacheByShard(0) })
	s.Panics(func() { state.StaleNansStateByShard(0) })
}

func (s *StateV2Suite) TestStatelessRelabelerTransition() {
	s.statelessRelabelerTransition(cppbridge.NewTransitionStateV2())
	s.statelessRelabelerTransition(cppbridge.NewTransitionStateV2WithoutLock())
}

func (s *StateV2Suite) statelessRelabelerTransition(state *cppbridge.StateV2) {
	s.Panics(func() { state.StatelessRelabeler() })

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler([]*cppbridge.RelabelConfig{})
	s.Require().NoError(err)

	s.Panics(func() { state.SetStatelessRelabeler(statelessRelabeler) })
	s.Panics(func() { state.StatelessRelabeler() })
}

func (s *StateV2Suite) createIdsMapping() *cppbridge.IdsMapping {
	lss1 := cppbridge.NewQueryableLssStorage()
	lss1.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "1").Build())
	snapshot := lss1.CreateLabelSetSnapshot()

	return snapshot.CopyAddedSeries(lss1.BitsetSeries(), cppbridge.NewQueryableLssStorage())
}

func (s *StateV2Suite) TestReconfigureWithInvalidOrderOfGenerationHead() {
	// Arrange
	state := cppbridge.NewStateV2()
	state.EnableTrackStaleness()
	state.Reconfigure(0, 1, nil)
	staleNanState := state.StaleNansStateByShard(0)

	// Act
	state.Reconfigure(2, 1, []*cppbridge.IdsMapping{s.createIdsMapping()})

	// Assert
	s.NotEqual(unsafe.Pointer(staleNanState), unsafe.Pointer(state.StaleNansStateByShard(0)))
}

func (s *StateV2Suite) TestReconfigureWithShardNumberChange() {
	// Arrange
	state := cppbridge.NewStateV2()
	state.EnableTrackStaleness()
	state.Reconfigure(0, 1, nil)
	staleNanState := state.StaleNansStateByShard(0)

	// Act
	state.Reconfigure(1, 2, []*cppbridge.IdsMapping{s.createIdsMapping()})

	// Assert
	s.NotEqual(unsafe.Pointer(staleNanState), unsafe.Pointer(state.StaleNansStateByShard(0)))
}

func (s *StateV2Suite) TestReconfigureWithNilIdsMapping() {
	// Arrange
	state := cppbridge.NewStateV2()
	state.EnableTrackStaleness()
	state.Reconfigure(0, 1, nil)
	staleNanState := state.StaleNansStateByShard(0)

	// Act
	state.Reconfigure(1, 2, []*cppbridge.IdsMapping{nil, nil})

	// Assert
	s.NotEqual(unsafe.Pointer(staleNanState), unsafe.Pointer(state.StaleNansStateByShard(0)))
}

func (s *StateV2Suite) TestReconfigureWithEmptyIdsMapping() {
	// Arrange
	state := cppbridge.NewStateV2()
	state.EnableTrackStaleness()
	state.Reconfigure(0, 1, nil)
	staleNanState := state.StaleNansStateByShard(0)

	// Act
	state.Reconfigure(1, 1, []*cppbridge.IdsMapping{{}, {}})

	// Assert
	s.NotEqual(unsafe.Pointer(staleNanState), unsafe.Pointer(state.StaleNansStateByShard(0)))
}

func (s *StateV2Suite) TestReconfigureWithRemap() {
	// Arrange
	state := cppbridge.NewStateV2()
	state.EnableTrackStaleness()
	state.Reconfigure(0, 2, nil)
	staleNanState1 := state.StaleNansStateByShard(0)
	staleNanState2 := state.StaleNansStateByShard(1)

	mapping := s.createIdsMapping()

	// Act
	state.Reconfigure(1, 2, []*cppbridge.IdsMapping{mapping, mapping})

	// Assert
	s.Equal(unsafe.Pointer(staleNanState1), unsafe.Pointer(state.StaleNansStateByShard(0)))
	s.Equal(unsafe.Pointer(staleNanState2), unsafe.Pointer(state.StaleNansStateByShard(1)))
}

func (s *RelabelerSuite) TestInputPerGoroutineRelabeler() {
	wr := prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	data, err := wr.Marshal()
	s.Require().NoError(err)

	rCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"job"},
			Regex:        "abc",
			Action:       cppbridge.Keep,
		},
	}

	inputLss := cppbridge.NewLssStorage()
	targetLss := cppbridge.NewQueryableLssStorage()

	var numberOfShards uint16 = 1

	hlimits := cppbridge.DefaultWALHashdexLimits()
	h, err := cppbridge.NewWALSnappyProtobufHashdex(snappy.Encode(nil, data), hlimits)
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(numberOfShards)

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(rCfgs)
	s.Require().NoError(err)

	state := cppbridge.NewStateV2WithoutLock()
	state.SetRelabelerOptions(&s.options)
	state.SetStatelessRelabeler(statelessRelabeler)
	state.Reconfigure(0, numberOfShards)

	pgr := cppbridge.NewPerGoroutineRelabeler(numberOfShards, 0)
	stats, hasReallocations, err := pgr.InputRelabeling(
		s.baseCtx,
		inputLss,
		targetLss,
		state,
		h,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 1, 1}, stats)
	s.True(hasReallocations)
}

func (s *RelabelerSuite) TestInputPerGoroutineRelabelerFromCacheTrue() {
	wr := prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	data, err := wr.Marshal()
	s.Require().NoError(err)

	rCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"job"},
			Regex:        "abc",
			Action:       cppbridge.Keep,
		},
	}

	inputLss := cppbridge.NewLssStorage()
	targetLss := cppbridge.NewQueryableLssStorage()

	var numberOfShards uint16 = 1

	hlimits := cppbridge.DefaultWALHashdexLimits()
	h, err := cppbridge.NewWALSnappyProtobufHashdex(snappy.Encode(nil, data), hlimits)
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(numberOfShards)

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(rCfgs)
	s.Require().NoError(err)

	state := cppbridge.NewStateV2WithoutLock()
	state.SetStatelessRelabeler(statelessRelabeler)
	state.SetRelabelerOptions(&s.options)
	state.Reconfigure(0, numberOfShards)

	pgr := cppbridge.NewPerGoroutineRelabeler(numberOfShards, 0)
	stats, hasReallocations, err := pgr.InputRelabeling(
		s.baseCtx,
		inputLss,
		targetLss,
		state,
		h,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 1, 0}, stats)
	s.True(hasReallocations)

	stats, ok, err := pgr.InputRelabelingFromCache(
		s.baseCtx,
		inputLss,
		targetLss,
		state,
		h,
		shardsInnerSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 0, 0}, stats)
	s.True(ok)
}

func (s *RelabelerSuite) TestInputPerGoroutineRelabelerFromCacheFalse() {
	wr := prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: time.Now().UnixMilli()},
				},
			},
		},
	}
	data, err := wr.Marshal()
	s.Require().NoError(err)

	inputLss := cppbridge.NewLssStorage()
	targetLss := cppbridge.NewQueryableLssStorage()

	var numberOfShards uint16 = 1

	hlimits := cppbridge.DefaultWALHashdexLimits()
	h, err := cppbridge.NewWALSnappyProtobufHashdex(snappy.Encode(nil, data), hlimits)
	s.Require().NoError(err)

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(numberOfShards)
	state := cppbridge.NewStateV2WithoutLock()
	state.SetRelabelerOptions(&s.options)
	state.Reconfigure(0, numberOfShards)

	pgr := cppbridge.NewPerGoroutineRelabeler(numberOfShards, 0)
	stats, ok, err := pgr.InputRelabelingFromCache(
		s.baseCtx,
		inputLss,
		targetLss,
		state,
		h,
		shardsInnerSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{0, 0, 0}, stats)
	s.False(ok)
}

func (s *RelabelerSuite) TestInputPerGoroutineRelabelerFromCachePartially() {
	ts := time.Now().UnixMilli()
	wr1 := prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value0"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value0"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value1"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value2"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value2"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value3"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value3"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value3"},
					{Name: "instance", Value: "value3"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value4"},
					{Name: "instance", Value: "value4"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value5"},
					{Name: "instance", Value: "value5"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value6"},
					{Name: "instance", Value: "value6"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
		},
	}
	data1, err := wr1.Marshal()
	s.Require().NoError(err)
	hlimits := cppbridge.DefaultWALHashdexLimits()
	h1, err := cppbridge.NewWALSnappyProtobufHashdex(snappy.Encode(nil, data1), hlimits)
	s.Require().NoError(err)

	ts += 6000
	wr2 := prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value0"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value0"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value1"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value1"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value2"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value2"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value3"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value3"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "value4"},
					{Name: "job", Value: "abc"},
					{Name: "instance", Value: "value4"},
				},
				Samples: []prompb.Sample{
					{Value: 0.1, Timestamp: ts},
				},
			},
		},
	}
	data2, err := wr2.Marshal()
	s.Require().NoError(err)
	h2, err := cppbridge.NewWALSnappyProtobufHashdex(snappy.Encode(nil, data2), hlimits)
	s.Require().NoError(err)

	rCfgs := []*cppbridge.RelabelConfig{
		{
			SourceLabels: []string{"job"},
			Regex:        "abc",
			Action:       cppbridge.Keep,
		},
	}

	inputLss := cppbridge.NewLssStorage()
	targetLss := cppbridge.NewQueryableLssStorage()

	var numberOfShards uint16 = 1

	shardsInnerSeries := cppbridge.NewShardsInnerSeries(numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(numberOfShards)

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(rCfgs)
	s.Require().NoError(err)

	state := cppbridge.NewStateV2WithoutLock()
	state.SetRelabelerOptions(&s.options)
	state.SetStatelessRelabeler(statelessRelabeler)
	state.Reconfigure(0, numberOfShards)

	pgr := cppbridge.NewPerGoroutineRelabeler(numberOfShards, 0)
	stats, hasReallocations, err := pgr.InputRelabeling(
		s.baseCtx,
		inputLss,
		targetLss,
		state,
		h1,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{4, 4, 4}, stats)
	s.True(hasReallocations)

	shardsInnerSeries = cppbridge.NewShardsInnerSeries(numberOfShards)
	stats, ok, err := pgr.InputRelabelingFromCache(
		s.baseCtx,
		inputLss,
		targetLss,
		state,
		h2,
		shardsInnerSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{4, 0, 0}, stats)
	s.False(ok)
	s.Equal(uint64(4), shardsInnerSeries[0].Size())

	stats, _, err = pgr.InputRelabeling(
		s.baseCtx,
		inputLss,
		targetLss,
		state,
		h2,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{1, 1, 0}, stats)
	s.Equal(uint64(5), shardsInnerSeries[0].Size())
}
