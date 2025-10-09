package config

import "github.com/prometheus/prometheus/pp/go/cppbridge"

//
// ScrapeConfig
//

// PPMetricRelabelConfigs returns slice the converted [relabel.Config] to the [cppbridge.RelabelConfig]'s.
func (c *ScrapeConfig) PPMetricRelabelConfigs() ([]*cppbridge.RelabelConfig, error) {
	cfgs, err := convertingRelabelConfigs(c.MetricRelabelConfigs)
	if err != nil {
		return nil, err
	}

	return cfgs, nil
}
