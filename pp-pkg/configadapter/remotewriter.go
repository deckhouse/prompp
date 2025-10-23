package configadapter

import (
	"crypto/md5" // #nosec G501 // cryptographic strength is not required
	"encoding/hex"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/remotewriter"
)

// RemoteWriterApplyConfig returns func-adapter for apply config on [remotewriter.RemoteWriter].
func RemoteWriterApplyConfig(remoteWriter *remotewriter.RemoteWriter) func(promConfig *config.Config) error {
	return func(promConfig *config.Config) error {
		destinationConfigs := make([]remotewriter.DestinationConfig, 0, len(promConfig.RemoteWriteConfigs))
		for _, rwc := range promConfig.RemoteWriteConfigs {
			if !rwc.IsPrometheusProtocol() {
				continue
			}

			if rwc.Name == "" {
				rwcHash, err := toHash(rwc)
				if err != nil {
					return err
				}
				rwc.Name = rwcHash
			}

			if rwc.QueueConfig.SampleAgeLimit == 0 {
				rwc.QueueConfig.SampleAgeLimit = remotewriter.DefaultSampleAgeLimit
			}

			destinationConfigs = append(destinationConfigs, remotewriter.DestinationConfig{
				RemoteWriteConfig: rwc.RemoteWriteConfig,
				ExternalLabels:    promConfig.GlobalConfig.ExternalLabels,
				ReadTimeout:       time.Duration(promConfig.GlobalConfig.ScrapeInterval),
			},
			)
		}
		return remoteWriter.ApplyConfig(destinationConfigs...)
	}
}

// toHash used for hashing configs and diff'ing hashes in ApplyConfig.
func toHash(data interface{}) (string, error) {
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}
	hash := md5.Sum(bytes) // #nosec G401 // cryptographic strength is not required
	return hex.EncodeToString(hash[:]), nil
}
