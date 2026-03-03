package ipfslite

import (
	"context"

	exchange "github.com/ipfs/boxo/exchange"
	provider "github.com/ipfs/boxo/provider"
	blocks "github.com/ipfs/go-block-format"
)

// providingExchange wraps an exchange and announces every newly added block.
type providingExchange struct {
	exchange.Interface
	provider provider.Provider
}

func newProvidingExchange(base exchange.Interface, provider provider.Provider) *providingExchange {
	return &providingExchange{
		Interface: base,
		provider:  provider,
	}
}

func (ex *providingExchange) NotifyNewBlocks(ctx context.Context, blocks ...blocks.Block) error {
	if err := ex.Interface.NotifyNewBlocks(ctx, blocks...); err != nil {
		return err
	}

	for _, block := range blocks {
		if err := ex.provider.Provide(ctx, block.Cid(), true); err != nil {
			return err
		}
	}

	return nil
}
