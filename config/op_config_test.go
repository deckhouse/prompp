// Copyright OpCore

package config_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/alecthomas/units"
	"github.com/go-kit/log"
	"github.com/odarix/odarix-core-go/cppbridge"
	"github.com/odarix/odarix-core-go/relabeler"
	common_config "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	op_config "github.com/prometheus/prometheus/op-pkg/config"
)

type OpConfigSuite struct {
	suite.Suite

	cfg *config.OpRemoteWriteConfig
}

func TestOpRemoteWriteConfig(t *testing.T) {
	suite.Run(t, new(OpConfigSuite))
}

func (s *OpConfigSuite) SetupTest() {
	s.cfg = new(config.OpRemoteWriteConfig)
}

func (s *OpConfigSuite) TestRemoteWriteConfig() {
	raw := `url: http://remote1/push
remote_timeout: 30s
write_relabel_configs:
- source_labels: [__name__]
  regex: expensive.*
  action: drop
name: drop_expensive
oauth2:
  client_id: "123"
  client_secret: "456"
  token_url: "http://remote1/auth"
  tls_config:
    cert_file: valid_cert_file
    key_file: valid_key_file`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().NoError(err)

	s.Require().Empty(s.cfg.Destinations)
	s.Require().Equal(config.PrometheusProtocol, s.cfg.Protocol)
	s.Require().Equal("http://remote1/push", s.cfg.URL.String())
}

func (s *OpConfigSuite) TestRemoteWriteConfigURLError() {
	raw := `remote_timeout: 30s
write_relabel_configs:
- source_labels: [__name__]
  regex: expensive.*
  action: drop
name: drop_expensive
oauth2:
  client_id: "123"
  client_secret: "456"
  token_url: "http://remote1/auth"
  tls_config:
    cert_file: valid_cert_file
    key_file: valid_key_file`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().Error(err)
}

func (s *OpConfigSuite) TestOpDestinationConfigURLError() {
	raw := `destinations:
- name: dname
remote_timeout: 30s
write_relabel_configs:
- source_labels: [__name__]
  regex: expensive.*
  action: drop`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().Error(err)
}

func (s *OpConfigSuite) TestOpDestinationConfigTLSError() {
	raw := `destinations:
- name: dname
  url: https://host.com
remote_timeout: 30s
write_relabel_configs:
- source_labels: [__name__]
  regex: expensive.*
  action: drop`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().Error(err)
}

func (s *OpConfigSuite) TestOpDestinationConfigError() {
	raw := `destinations:
- name: dname
  url: https://host.com
  tls_config:
    server_name: server_name
remote_timeout: 30s
write_relabel_configs:
- source_labels: [__name__]
  regex: expensive.*
  action: drop`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().NoError(err)
	s.Require().NotEmpty(s.cfg.Destinations)
	s.Require().Equal("server_name", s.cfg.Destinations[0].HTTPClientConfig.TLSConfig.ServerName)
}

func (s *OpConfigSuite) TestLoadConfig() {
	// Parse a valid file that sets a global scrape timeout. This tests whether parsing
	// an overwritten default field in the global config permanently changes the default.
	_, err := config.LoadFile("testdata/global_timeout.good.yml", false, false, log.NewNopLogger())
	s.Require().NoError(err)

	c, err := config.LoadFile("testdata/op.conf.good.yml", false, false, log.NewNopLogger())
	s.Require().NoError(err)
	s.Require().Equal(expectedConf, c)
}

func mustParseURL(u string) *common_config.URL {
	parsed, err := url.Parse(u)
	if err != nil {
		panic(err)
	}
	return &common_config.URL{URL: parsed}
}

var expectedConf = &config.Config{
	GlobalConfig: config.GlobalConfig{
		ScrapeInterval:     model.Duration(15 * time.Second),
		ScrapeTimeout:      config.DefaultGlobalConfig.ScrapeTimeout,
		EvaluationInterval: model.Duration(30 * time.Second),
		QueryLogFile:       "",

		ExternalLabels: labels.FromStrings("foo", "bar", "monitor", "codelab"),

		BodySizeLimit:         15 * units.MiB,
		SampleLimit:           1500,
		TargetLimit:           30,
		LabelLimit:            30,
		LabelNameLengthLimit:  200,
		LabelValueLengthLimit: 200,
		ScrapeProtocols:       config.DefaultGlobalConfig.ScrapeProtocols,
	},

	RemoteWriteConfigs: []*config.OpRemoteWriteConfig{
		{
			Protocol: config.PrometheusProtocol,
			Destinations: []*config.OpDestinationConfig{
				{
					Name: "dname",
					URL:  mustParseURL("https://host.com"),
					HTTPClientConfig: common_config.HTTPClientConfig{
						TLSConfig: common_config.TLSConfig{
							ServerName: "server_name",
						},
					},
				},
			},
			RemoteWriteConfig: config.RemoteWriteConfig{
				RemoteTimeout:  model.Duration(30 * time.Second),
				QueueConfig:    config.DefaultQueueConfig,
				MetadataConfig: config.DefaultMetadataConfig,
				HTTPClientConfig: common_config.HTTPClientConfig{
					FollowRedirects: true,
					EnableHTTP2:     true,
				},
			},
		},
	},

	ReceiverConfig: op_config.RemoteWriteReceiverConfig{
		NumberOfShards: 2,
		Configs: []*relabeler.InputRelabelerConfig{
			{
				Name: "some_remote_write_receiver_1",
				RelabelConfigs: []*cppbridge.RelabelConfig{
					{
						SourceLabels: []string{"__name__"},
						Separator:    ";",
						Regex:        ".*",
						Replacement:  "$1",
						Action:       cppbridge.Keep,
					},
				},
			},
		},
	},
}
