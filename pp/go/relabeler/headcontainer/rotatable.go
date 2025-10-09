package headcontainer

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
)

// CopySeriesOnRotate copy active series from the current head to the new head during rotation.
var CopySeriesOnRotate = false

// Storage - head storage.
type Storage interface {
	Add(head relabeler.Head)
}

// HeadBuilder - head builder.
type HeadBuilder interface {
	Build() (relabeler.Head, error)
	BuildWithConfig(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) (relabeler.Head, error)
}

type HeadActivator interface {
	Activate(headID string) error
}

type NoOpHeadActivator struct{}

func (NoOpHeadActivator) Activate(headID string) error { return nil }

// Rotatable head wrapper, allows rotations.
type Rotatable struct {
	storage       Storage
	builder       HeadBuilder
	headActivator HeadActivator
	relabeler.Head
}

// NewRotatable init new [*Rotatable] container head.
func NewRotatable(
	head relabeler.Head,
	storage Storage,
	builder HeadBuilder,
	headActivator HeadActivator,
) *Rotatable {
	return &Rotatable{
		storage:       storage,
		builder:       builder,
		headActivator: headActivator,
		Head:          head,
	}
}

// Reconfigure relabeler.Head interface implementation.
func (h *Rotatable) Reconfigure(
	ctx context.Context,
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) error {
	if h.Head.NumberOfShards() != numberOfShards {
		return h.RotateWithConfig(inputRelabelerConfigs, numberOfShards)
	}
	return h.Head.Reconfigure(ctx, inputRelabelerConfigs, numberOfShards)
}

// Rotate - relabeler.Head interface implementation.
func (h *Rotatable) Rotate() error {
	h.Head.MergeOutOfOrderChunks()

	newHead, err := h.builder.Build()
	if err != nil {
		return err
	}

	if CopySeriesOnRotate {
		newHead.CopySeriesFrom(h.Head)
	}

	if err = h.headActivator.Activate(newHead.ID()); err != nil {
		return err
	}

	if err = h.Head.CommitToWal(); err != nil {
		logger.Errorf("failed to commit wal on rotation: %v", err)
	}
	h.Head.Stop()

	h.storage.Add(h.Head)
	h.Head = newHead
	return nil
}

func (h *Rotatable) RotateWithConfig(
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) error {
	h.Head.MergeOutOfOrderChunks()

	newHead, err := h.builder.BuildWithConfig(inputRelabelerConfigs, numberOfShards)
	if err != nil {
		return err
	}

	if err = h.headActivator.Activate(newHead.ID()); err != nil {
		return err
	}

	if err = h.Head.CommitToWal(); err != nil {
		logger.Errorf("failed to commit wal on rotation: %v", err)
	}
	h.Head.Stop()

	h.storage.Add(h.Head)
	h.Head = newHead
	return nil
}
