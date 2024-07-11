// Copyright OpCore

package config

import (
	"errors"
	"fmt"

	"github.com/prometheus/common/config"
)

const (
	PrometheusProtocol = "prometheus"
	ProtocolOdarix     = "odarix"
)

var (
	// DefaultOpRemoteWriteConfig is the default remote write configuration.
	DefaultOpRemoteWriteConfig = OpRemoteWriteConfig{
		Protocol:          PrometheusProtocol,
		RemoteWriteConfig: DefaultRemoteWriteConfig,
	}
	emptyTLSConfig = config.TLSConfig{}
)

// RemoteWriteConfig is the configuration for writing to remote storage.
type OpRemoteWriteConfig struct {
	Protocol          string                 `yaml:"protocol,omitempty"`
	Destinations      []*OpDestinationConfig `yaml:"destinations"`
	RemoteWriteConfig `yaml:",inline"`
}

// SetDirectory joins any relative file paths with dir.
func (c *OpRemoteWriteConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)

	if len(c.Destinations) == 0 {
		return
	}

	for _, d := range c.Destinations {
		d.HTTPClientConfig.SetDirectory(dir)
	}
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *OpRemoteWriteConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultOpRemoteWriteConfig
	type plain OpRemoteWriteConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return c.Validate()
}

// Validate validates OpRemoteWriteConfig, but also fills relevant default values from global config if needed.
func (c *OpRemoteWriteConfig) Validate() error {
	if len(c.Destinations) == 0 {
		return c.RemoteWriteConfig.Validate()
	}

	return nil
}

// Validate validates RemoteWriteConfig, but also fills relevant default values from global config if needed.
func (c *RemoteWriteConfig) Validate() error {
	if c.URL == nil {
		return errors.New("url for remote_write is empty")
	}
	for _, rlcfg := range c.WriteRelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null relabeling rule in remote write config")
		}
	}
	if err := validateHeaders(c.Headers); err != nil {
		return err
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	httpClientConfigAuthEnabled := c.HTTPClientConfig.BasicAuth != nil ||
		c.HTTPClientConfig.Authorization != nil || c.HTTPClientConfig.OAuth2 != nil

	if httpClientConfigAuthEnabled && (c.SigV4Config != nil || c.AzureADConfig != nil) {
		return fmt.Errorf("at most one of basic_auth, authorization, oauth2, sigv4, & azuread must be configured")
	}

	if c.SigV4Config != nil && c.AzureADConfig != nil {
		return fmt.Errorf("at most one of basic_auth, authorization, oauth2, sigv4, & azuread must be configured")
	}

	return nil
}

// OpDestinationConfig is the configuration for destination server.
type OpDestinationConfig struct {
	Name             string                  `yaml:"name"`
	URL              *config.URL             `yaml:"url"`
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *OpDestinationConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = OpDestinationConfig{}
	type plain OpDestinationConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return c.Validate()
}

// Validate validates OpDestinationConfig, but also fills relevant default values from global config if needed.
func (c *OpDestinationConfig) Validate() error {
	if c.Name == "" {
		return errors.New("destination name is empty")
	}

	if c.URL == nil {
		return errors.New("url for destination is empty")
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	if c.HTTPClientConfig.TLSConfig == emptyTLSConfig {
		return errors.New("tls config for destination is empty")
	}

	return nil
}
