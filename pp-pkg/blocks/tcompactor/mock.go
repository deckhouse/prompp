package tcompactor

import "github.com/prometheus/prometheus/pp-pkg/blocks/lcompactor"

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg mock --out
//go:generate moq mock/planner.go . Planner
//go:generate moq mock/compactor.go . Compactor
//go:generate moq mock/block_populator.go . BlockPopulator

// BlockPopulator is a alias for the [lcompactor.BlockPopulator] interface.
type BlockPopulator = lcompactor.BlockPopulator
