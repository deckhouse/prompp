// Copyright OpCore

package receiver

import (
	"context"
	"fmt"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/odarix/odarix-core-go/cppbridge"
	"github.com/odarix/odarix-core-go/relabeler"
	"github.com/prometheus/client_golang/prometheus"
	common_config "github.com/prometheus/common/config"
	"gopkg.in/yaml.v2"

	prom_config "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/relabel"
	op_config "github.com/prometheus/prometheus/op-pkg/config"
	"github.com/prometheus/prometheus/op-pkg/dialer"
)

type Receiver struct {
	managerRelabeler *relabeler.ManagerRelabeler
	hashdexFactory   relabeler.HashdexFactory
	hashdexLimits    cppbridge.WALHashdexLimits
	haTracker        relabeler.HATracker
	logger           log.Logger
}

func NewReceiver(
	ctx context.Context,
	logger log.Logger,
	registerer prometheus.Registerer,
	receiverCfg *op_config.RemoteWriteReceiverConfig,
	workingDir string,
	remoteWriteCfgs []*prom_config.OpRemoteWriteConfig,
) (*Receiver, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	initLogHandler(logger)
	clock := clockwork.NewRealClock()

	destinationGroups, err := makeDestinationGroups(
		ctx,
		clock,
		registerer,
		workingDir,
		remoteWriteCfgs,
		receiverCfg.NumberOfShards,
	)
	if err != nil {
		level.Error(logger).Log("msg", "failed to init DestinationGroups", "err", err)
		return nil, err
	}

	mr, err := relabeler.NewManagerRelabeler(
		clock,
		registerer,
		receiverCfg.Configs,
		destinationGroups,
		receiverCfg.NumberOfShards,
	)
	if err != nil {
		return nil, err
	}

	return &Receiver{
		managerRelabeler: mr,
		hashdexFactory:   cppbridge.HashdexFactory{},
		hashdexLimits:    cppbridge.DefaultWALHashdexLimits(),
		haTracker:        relabeler.NewHighAvailabilityTracker(ctx, registerer, clock),
		logger:           logger,
	}, nil
}

// Append append to relabeling heshdex data.
func (rr *Receiver) Append(ctx context.Context, data relabeler.ProtoData, relabelerID string) error {
	hx, err := rr.hashdexFactory.Protobuf(data.Bytes(), rr.hashdexLimits)
	if err != nil {
		data.Destroy()
		return err
	}

	if rr.haTracker.IsDrop(hx.Cluster(), hx.Replica()) {
		data.Destroy()
		return nil
	}
	incomingData := &relabeler.IncomingData{Hashdex: hx, Data: data}
	return rr.managerRelabeler.Append(ctx, incomingData, relabelerID)
}

// ApplyConfig update config.
func (rr *Receiver) ApplyConfig(inputRelabelerConfigs []*relabeler.InputRelabelerConfig, numberOfShards uint16) error {
	return rr.managerRelabeler.ApplyConfig(inputRelabelerConfigs, numberOfShards)
}

// Run main loop.
func (rr *Receiver) Run(ctx context.Context) {
	rr.managerRelabeler.Run(ctx)
}

// Shutdown safe shutdown Receiver.
func (rr *Receiver) Shutdown(ctx context.Context) error {
	return rr.managerRelabeler.Shutdown(ctx)
}

// makeDestinationGroups create DestinationGroups from configs.
func makeDestinationGroups(
	ctx context.Context,
	clock clockwork.Clock,
	registerer prometheus.Registerer,
	workingDir string,
	rwCfgs []*prom_config.OpRemoteWriteConfig,
	numberOfShards uint16,
) (*relabeler.DestinationGroups, error) {
	dgs := make(relabeler.DestinationGroups, 0, len(rwCfgs))

	for _, rwCfg := range rwCfgs {
		dgCfg, err := makeDestinationGroupConfig(rwCfg, workingDir, numberOfShards)
		if err != nil {
			return nil, err
		}

		// TODO ClientID
		dialersConfigs, err := makeConfigDialers(rwCfg.Name, rwCfg.Destinations)
		if err != nil {
			return nil, err
		}
		dialers, err := makeDialers(clock, registerer, dialersConfigs)
		if err != nil {
			return nil, err
		}

		dg, err := relabeler.NewDestinationGroup(
			ctx,
			dgCfg,
			encoderCtor,
			refillCtor,
			refillSenderCtor,
			clock,
			dialers,
			registerer,
		)
		if err != nil {
			return nil, err
		}

		dgs = append(dgs, dg)
	}

	return &dgs, nil
}

// makeDestinationGroupConfig converting incoming config to internal DestinationGroupConfig.
func makeDestinationGroupConfig(
	rwCfg *prom_config.OpRemoteWriteConfig,
	workingDir string,
	numberOfShards uint16,
) (*relabeler.DestinationGroupConfig, error) {
	rCfgs, err := convertingRelabelersConfig(rwCfg.WriteRelabelConfigs)
	if err != nil {
		return nil, err
	}

	dgcfg := relabeler.NewDestinationGroupConfig(
		rwCfg.Name,
		workingDir,
		rCfgs,
		numberOfShards,
	)

	return dgcfg, nil
}

// convertingRelabelersConfig converting incoming relabel config to internal relabel config.
func convertingRelabelersConfig(rCfgs []*relabel.Config) ([]*cppbridge.RelabelConfig, error) {
	var crCfgs []*cppbridge.RelabelConfig
	raw, err := yaml.Marshal(rCfgs)
	if err != nil {
		return nil, err
	}

	if err = yaml.Unmarshal(raw, &crCfgs); err != nil {
		return nil, err
	}

	return crCfgs, nil
}

// makeConfigDialers converting and make internal dialer configs.
func makeConfigDialers(
	clientID string,
	sCfgs []*prom_config.OpDestinationConfig,
) ([]*relabeler.DialersConfig, error) {
	dialersConfigs := make([]*relabeler.DialersConfig, 0, len(sCfgs))
	for _, sCfg := range sCfgs {
		tlsCfg, err := common_config.NewTLSConfig(&sCfg.HTTPClientConfig.TLSConfig)
		if err != nil {
			return nil, err
		}
		ccfg := dialer.NewCommonConfig(
			sCfg.URL.URL,
			tlsCfg,
			sCfg.Name,
		)
		dialersConfigs = append(
			dialersConfigs,
			&relabeler.DialersConfig{
				DialerConfig: relabeler.NewDialerConfig(
					sCfg.URL.URL,
					clientID,
					string(sCfg.HTTPClientConfig.BearerToken),
				),
				ConnDialerConfig: ccfg,
			},
		)
	}

	return dialersConfigs, nil
}

// makeDialers create dialers from main config according to the specified parameters.
func makeDialers(
	clock clockwork.Clock,
	registerer prometheus.Registerer,
	dialersConfig []*relabeler.DialersConfig,
) ([]relabeler.Dialer, error) {
	dialers := make([]relabeler.Dialer, 0, len(dialersConfig))
	for i := range dialersConfig {
		ccfg, ok := dialersConfig[i].ConnDialerConfig.(*dialer.CommonConfig)
		if !ok {
			return nil, fmt.Errorf("invalid CommonConfig: %v", dialersConfig[i].ConnDialerConfig)
		}

		d := dialer.DefaultDialer(ccfg, registerer)

		tcpDialer := relabeler.NewWebSocketDialer(
			d,
			dialersConfig[i].DialerConfig,
			clock,
			registerer,
		)
		dialers = append(dialers, tcpDialer)
	}

	return dialers, nil
}

// encoderCtor default contructor for encoder.
func encoderCtor(shardID uint16, shardsNumberPower uint8) relabeler.ManagerEncoder {
	return cppbridge.NewWALEncoderLightweight(shardID, shardsNumberPower)
}

// refillCtor default contructor for refill.
func refillCtor(
	workinDir string,
	blockID uuid.UUID,
	destinations []string,
	shardsNumberPower uint8,
	segmentEncodingVersion uint8,
	alwaysToRefill bool,
	name string,
	registerer prometheus.Registerer,
) (relabeler.ManagerRefill, error) {
	return relabeler.NewRefill(
		workinDir,
		shardsNumberPower,
		segmentEncodingVersion,
		blockID,
		alwaysToRefill,
		name,
		registerer,
		destinations...,
	)
}

// refillSenderCtor default contructor for manager sender.
func refillSenderCtor(
	rsmCfg relabeler.RefillSendManagerConfig,
	workingDir string,
	dialers []relabeler.Dialer,
	clock clockwork.Clock,
	name string,
	registerer prometheus.Registerer,
) (relabeler.ManagerRefillSender, error) {
	return relabeler.NewRefillSendManager(rsmCfg, workingDir, dialers, clock, name, registerer)
}

// initLogHandler init log handler for ManagerKeeper.
func initLogHandler(logger log.Logger) {
	relabeler.Debugf = func(template string, args ...interface{}) {
		level.Debug(logger).Log("msg", fmt.Sprintf(template, args...))
	}
	relabeler.Infof = func(template string, args ...interface{}) {
		level.Info(logger).Log("msg", fmt.Sprintf(template, args...))
	}
	relabeler.Warnf = func(template string, args ...interface{}) {
		level.Warn(logger).Log("msg", fmt.Sprintf(template, args...))
	}
	relabeler.Errorf = func(template string, args ...interface{}) {
		level.Error(logger).Log("msg", fmt.Sprintf(template, args...))
	}
}
