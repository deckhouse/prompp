package configadapter

import (
	prom_config "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pp/go/storage"
)

// HeadKeeperApplyConfig returns func-adapter for apply config on [headkeeper.HeadKeeper].
func HeadKeeperApplyConfig(m *storage.Manager) func(cfg *prom_config.Config) error {
	return func(cfg *prom_config.Config) error {
		rCfg, err := cfg.GetReceiverConfig()
		if err != nil {
			return err
		}

		return m.ApplyConfig(rCfg.NumberOfShards)
	}
}
