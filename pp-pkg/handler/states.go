package handler

import (
	"fmt"
	"sync"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

//
// States
//

// void empty struct.
var void = struct{}{}

// StatesStorage stores the [cppbridge.State]'s.
type StatesStorage struct {
	m  map[string]*cppbridge.StateV2
	mx sync.RWMutex
}

// NewStatesStorage init new [StatesStorage].
func NewStatesStorage() *StatesStorage {
	return &StatesStorage{
		m:  map[string]*cppbridge.StateV2{config.TransparentRelabeler: cppbridge.NewTransitionStateV2()},
		mx: sync.RWMutex{},
	}
}

// ApplyConfig updates the [StatesStorage]'s configs.
func (s *StatesStorage) ApplyConfig(conf *config.Config) error {
	rwcfgs := conf.RemoteWriteReceiverConfig()
	if len(rwcfgs.Configs) == 0 {
		return nil
	}

	updated := make(map[string]struct{}, len(rwcfgs.Configs)+1)
	updated[config.TransparentRelabeler] = void

	s.mx.Lock()
	defer s.mx.Unlock()
	for _, cfg := range rwcfgs.Configs {
		stateID := cfg.GetName()
		rcfg := cfg.GetConfigs()

		if st, ok := s.m[stateID]; ok {
			if st.StatelessRelabeler().EqualConfigs(rcfg) {
				updated[stateID] = void
				continue
			}
		}

		statelessRelabeler, err := cppbridge.NewStatelessRelabeler(rcfg)
		if err != nil {
			return fmt.Errorf("failed creating stateless relabeler for %s: %w", stateID, err)
		}

		state := cppbridge.NewStateV2()
		state.SetStatelessRelabeler(statelessRelabeler)

		s.m[stateID] = state
		updated[stateID] = void
	}

	for stateID := range s.m {
		if _, ok := updated[stateID]; !ok {
			// clear unnecessary
			delete(s.m, stateID)
		}
	}

	return nil
}

// GetStateByID returns [cppbridge.State] by state ID if exist.
func (s *StatesStorage) GetStateByID(stateID string) (*cppbridge.StateV2, bool) {
	s.mx.RLock()
	state, ok := s.m[stateID]
	s.mx.RUnlock()

	return state, ok
}
