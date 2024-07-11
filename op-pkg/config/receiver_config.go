// Copyright OpCore

package config

import (
	"fmt"

	"github.com/odarix/odarix-core-go/relabeler"
)

// RemoteWriteReceiverConfig config for remote write receiver.
type RemoteWriteReceiverConfig struct {
	NumberOfShards uint16                            `yaml:"number_of_shards,omitempty"`
	Configs        []*relabeler.InputRelabelerConfig `yaml:"remote_write_receivers,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *RemoteWriteReceiverConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain RemoteWriteReceiverConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if len(c.Configs) != 0 && c.NumberOfShards == 0 {
		c.NumberOfShards = 2
	}

	return c.Validate()
}

func (c *RemoteWriteReceiverConfig) Validate() error {
	rNames := map[string]struct{}{}
	for _, rcfg := range c.Configs {
		if _, ok := rNames[rcfg.Name]; ok {
			return fmt.Errorf("found multiple input relabeler configs with job name %q", rcfg.Name)
		}
		rNames[rcfg.Name] = struct{}{}
	}

	return nil
}
