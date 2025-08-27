package configadapter

import (
	"context"

	prom_config "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pp/go/storage/head/manager"
)

// DefaultNumberOfShards default value for number of shards [pp_storage.Head].
var DefaultNumberOfShards uint16 = 2

// HeadKeeperApplyConfig returns func-adapter for apply config on [headkeeper.HeadKeeper].
func HeadKeeperApplyConfig[THead any](
	ctx context.Context,
	hk *manager.Manager[THead],
) func(cfg *prom_config.Config) error {
	return func(cfg *prom_config.Config) error {
		rCfg, err := cfg.GetReceiverConfig()
		if err != nil {
			return err
		}

		numberOfShards := rCfg.NumberOfShards
		if numberOfShards == 0 {
			numberOfShards = DefaultNumberOfShards
		}

		return hk.ApplyConfig(ctx, numberOfShards)
	}
}
