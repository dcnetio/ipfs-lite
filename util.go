package ipfslite

import (
	"context"
	"io"
	"time"

	ipns "github.com/ipfs/boxo/ipns"
	datastore "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	dualdht "github.com/libp2p/go-libp2p-kad-dht/dual"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/pnet"
	"github.com/libp2p/go-libp2p/core/routing"
	libp2pYamux "github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/libp2p/go-libp2p/p2p/transport/websocket"
	yamuxv5 "github.com/libp2p/go-yamux/v5"
	"github.com/multiformats/go-multiaddr"
)

// DefaultBootstrapPeers returns the default bootstrap peers (for use
// with NewLibp2pHost.
func DefaultBootstrapPeers() []peer.AddrInfo {
	peers, _ := peer.AddrInfosFromP2pAddrs(dht.DefaultBootstrapPeers...)
	return peers
}

// NewInMemoryDatastore provides a sync datastore that lives in-memory only
// and is not persisted.
func NewInMemoryDatastore() datastore.Batching {
	return dssync.MutexWrap(datastore.NewMapDatastore())
}

var connMgr, _ = connmgr.NewConnManager(100, 600, connmgr.WithGracePeriod(time.Minute))

// Libp2pOptionsExtra provides some useful libp2p options
// to create a fully featured libp2p host. It can be used with
// SetupLibp2p.
var Libp2pOptionsExtra = []libp2p.Option{
	libp2p.NATPortMap(),
	libp2p.ConnectionManager(connMgr),
	libp2p.EnableAutoRelayWithPeerSource(func(_ context.Context, num int) <-chan peer.AddrInfo {
		peerChan := make(chan peer.AddrInfo, num)
		defer close(peerChan)
		ipfspeers := DefaultBootstrapPeers()
		for i := 0; i < num && i < len(ipfspeers); i++ {
			peerChan <- ipfspeers[i]
		}
		return peerChan
	}),
	//}, time.Minute)),
	libp2p.EnableNATService(),
}

// SetupLibp2p returns a routed host and DHT instances that can be used to
// easily create a ipfslite Peer. You may consider to use Peer.Bootstrap()
// after creating the IPFS-Lite Peer to connect to other peers. When the
// datastore parameter is nil, the DHT will use an in-memory datastore, so all
// provider records are lost on program shutdown.
//
// Additional libp2p options can be passed. Note that the Identity,
// ListenAddrs and PrivateNetwork options will be setup automatically.
// Interesting options to pass: NATPortMap() EnableAutoRelay(),
// libp2p.EnableNATService(), DisableRelay(), ConnectionManager(...)... see
// https://godoc.org/github.com/libp2p/go-libp2p#Option for more info.
//
// The secret should be a 32-byte pre-shared-key byte slice.
func SetupLibp2p(
	ctx context.Context,
	hostKey crypto.PrivKey,
	secret pnet.PSK,
	listenAddrs []multiaddr.Multiaddr,
	ds datastore.Batching,
	dhtMode dht.ModeOpt,
	opts ...libp2p.Option,
) (host.Host, *dualdht.DHT, error) {

	var ddht *dualdht.DHT
	var err error
	var transports = libp2p.DefaultTransports

	if secret != nil {
		transports = libp2p.ChainOptions(
			libp2p.NoTransports,
			libp2p.Transport(tcp.NewTCPTransport),
			libp2p.Transport(websocket.New),
		)
	}

	finalOpts := []libp2p.Option{
		libp2p.Identity(hostKey),
		libp2p.ListenAddrs(listenAddrs...),
		libp2p.PrivateNetwork(secret),
		transports,
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			ddht, err = newDHT(ctx, h, ds, dhtMode)
			return ddht, err
		}),
	}
	finalOpts = append(finalOpts, opts...)

	// 关键参数调整
	yamuxConfig := yamuxv5.DefaultConfig()
	yamuxConfig.MaxStreamWindowSize = 1024 * 1024 * 4 // 单个流窗口 4MB
	yamuxConfig.AcceptBacklog = 128                   // 流接收队列长度
	yamuxConfig.EnableKeepAlive = true                // 开启保活
	yamuxConfig.KeepAliveInterval = 15 * time.Second  // 保活间隔
	yamuxConfig.MaxMessageSize = 16 * 1024 * 1024     // 最大消息大小
	yamuxConfig.LogOutput = io.Discard                // 日志输出
	yamuxTransport := libp2pYamux.Transport(*yamuxConfig)

	finalOpts = append(finalOpts, libp2p.Muxer("/yamux/1.0.0", &yamuxTransport))
	h, err := libp2p.New(
		finalOpts...,
	)
	if err != nil {
		return nil, nil, err
	}

	return h, ddht, nil
}

func newDHT(ctx context.Context, h host.Host, ds datastore.Batching, dhtMode dht.ModeOpt) (*dualdht.DHT, error) {
	dhtOpts := []dualdht.Option{
		dualdht.DHTOption(dht.NamespacedValidator("pk", record.PublicKeyValidator{})),
		dualdht.DHTOption(dht.NamespacedValidator("ipns", ipns.Validator{KeyBook: h.Peerstore()})),
		dualdht.DHTOption(dht.Concurrency(10)),
		dualdht.DHTOption(dht.Mode(dhtMode)),
	}
	if ds != nil {
		dhtOpts = append(dhtOpts, dualdht.DHTOption(dht.Datastore(ds)))
	}
	return dualdht.New(ctx, h, dhtOpts...)

}
