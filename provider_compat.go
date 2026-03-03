package ipfslite

import (
	"context"

	blockstore "github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/go-cid"
)

func newBlockstoreProvider(bstore blockstore.Blockstore) func(context.Context) (<-chan cid.Cid, error) {
	return func(ctx context.Context) (<-chan cid.Cid, error) {
		return bstore.AllKeysChan(ctx)
	}
}
