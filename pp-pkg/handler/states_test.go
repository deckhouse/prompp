package handler_test

import (
	"testing"

	"github.com/prometheus/prometheus/config"
	pp_pkg_config "github.com/prometheus/prometheus/pp-pkg/config"
	"github.com/prometheus/prometheus/pp-pkg/handler"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	rconfig "github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/stretchr/testify/suite"
)

type StatesStorageSuite struct {
	suite.Suite
}

func TestStatesStorageSuite(t *testing.T) {
	suite.Run(t, new(StatesStorageSuite))
}

func (s *StatesStorageSuite) TestHappyPath() {
	states := handler.NewStatesStorage()

	state, ok := states.GetStateByID(config.TransparentRelabeler)

	s.Require().True(ok)
	s.Require().NotNil(state)
	s.Require().True(state.IsTransition())
}

func (s *StatesStorageSuite) TestNotExist() {
	states := handler.NewStatesStorage()

	state, ok := states.GetStateByID("test")
	s.Require().False(ok)
	s.Require().Nil(state)
}

func (s *StatesStorageSuite) TestApplyConfigEmpty() {
	states := handler.NewStatesStorage()
	cfg := &config.Config{
		RemoteWriteConfigs: []*config.PPRemoteWriteConfig{},
	}

	states.ApplyConfig(cfg)

	state, ok := states.GetStateByID(config.TransparentRelabeler)
	s.Require().True(ok)
	s.Require().NotNil(state)
	s.Require().True(state.IsTransition())
}

func (s *StatesStorageSuite) TestApplyConfig() {
	states := handler.NewStatesStorage()
	cfg := &config.Config{
		ReceiverConfig: pp_pkg_config.RemoteWriteReceiverConfig{
			Configs: []*rconfig.InputRelabelerConfig{
				{
					Name:           "test",
					RelabelConfigs: []*cppbridge.RelabelConfig{},
				},
			},
		},
	}

	states.ApplyConfig(cfg)

	state, ok := states.GetStateByID(config.TransparentRelabeler)
	s.Require().True(ok)
	s.Require().NotNil(state)
	s.Require().True(state.IsTransition())

	state, ok = states.GetStateByID("test")
	s.Require().True(ok)
	s.Require().NotNil(state)
	s.Require().False(state.IsTransition())
}

func (s *StatesStorageSuite) TestApplyConfigDouble() {
	states := handler.NewStatesStorage()
	cfg := &config.Config{
		ReceiverConfig: pp_pkg_config.RemoteWriteReceiverConfig{
			Configs: []*rconfig.InputRelabelerConfig{
				{
					Name:           "test",
					RelabelConfigs: []*cppbridge.RelabelConfig{},
				},
			},
		},
	}

	states.ApplyConfig(cfg)

	state, ok := states.GetStateByID(config.TransparentRelabeler)
	s.Require().True(ok)
	s.Require().NotNil(state)
	s.Require().True(state.IsTransition())

	state, ok = states.GetStateByID("test")
	s.Require().True(ok)
	s.Require().NotNil(state)
	s.Require().False(state.IsTransition())

	states.ApplyConfig(cfg)

	state, ok = states.GetStateByID("test")
	s.Require().True(ok)
	s.Require().NotNil(state)
	s.Require().False(state.IsTransition())
}

func (s *StatesStorageSuite) TestApplyConfigDoubleChange() {
	states := handler.NewStatesStorage()
	cfg := &config.Config{
		ReceiverConfig: pp_pkg_config.RemoteWriteReceiverConfig{
			Configs: []*rconfig.InputRelabelerConfig{
				{
					Name:           "test",
					RelabelConfigs: []*cppbridge.RelabelConfig{},
				},
			},
		},
	}

	states.ApplyConfig(cfg)

	state, ok := states.GetStateByID(config.TransparentRelabeler)
	s.Require().True(ok)
	s.Require().NotNil(state)
	s.Require().True(state.IsTransition())

	state, ok = states.GetStateByID("test")
	s.Require().True(ok)
	s.Require().NotNil(state)
	s.Require().False(state.IsTransition())

	cfg = &config.Config{
		ReceiverConfig: pp_pkg_config.RemoteWriteReceiverConfig{
			Configs: []*rconfig.InputRelabelerConfig{
				{
					Name:           "test2",
					RelabelConfigs: []*cppbridge.RelabelConfig{},
				},
			},
		},
	}

	states.ApplyConfig(cfg)

	state, ok = states.GetStateByID("test")
	s.Require().False(ok)
	s.Require().Nil(state)

	state, ok = states.GetStateByID("test2")
	s.Require().True(ok)
	s.Require().NotNil(state)
	s.Require().False(state.IsTransition())
}
