package head

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
)

type ConfigSource interface {
	Config() (inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16)
}

type ConfigSourceFunc func() (inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16)

func (fn ConfigSourceFunc) Config() (inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) {
	return fn()
}

type BuildFunc func(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) (relabeler.Head, error)

type BuildWithLSSFunc func(
	targetLsses []*cppbridge.LabelSetStorage,
	inputRelabelerConfigs []*config.InputRelabelerConfig,
) (relabeler.Head, error)

type Builder struct {
	configSource     ConfigSource
	buildFunc        BuildFunc
	buildWithLSSFunc BuildWithLSSFunc
}

func NewBuilder(configSource ConfigSource, buildFunc BuildFunc, buildWithLSSFunc BuildWithLSSFunc) *Builder {
	return &Builder{
		configSource:     configSource,
		buildFunc:        buildFunc,
		buildWithLSSFunc: buildWithLSSFunc,
	}
}

// BuildWithConfig build head with incoming config.
func (b *Builder) BuildWithConfig(
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) (relabeler.Head, error) {
	return b.buildFunc(inputRelabelerConfigs, numberOfShards)
}

// BuildWithLSS head with target lsses.
func (b *Builder) BuildWithLSS(targetLsses []*cppbridge.LabelSetStorage) (relabeler.Head, error) {
	inputRelabelerConfigs, numberOfShards := b.configSource.Config()
	if uint16(len(targetLsses)) != numberOfShards { //nolint:gosec // no overflow
		return b.buildFunc(inputRelabelerConfigs, numberOfShards)
	}

	return b.buildWithLSSFunc(targetLsses, inputRelabelerConfigs)
}
