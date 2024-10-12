package p2p

import (
	"context"
	"fmt"
	"github.com/ipfs/boxo/ipns"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	dualdht "github.com/libp2p/go-libp2p-kad-dht/dual"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/routing"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	"log"
	"strings"
	"time"
)

type LocalNode struct {
	Host        host.Host
	KadDHT      *dht.IpfsDHT
	DualDht     *dualdht.DHT
	PingService *ping.PingService
}

func NewLocalNode(ctx context.Context, priv crypto.PrivKey, p2pPort int) (*LocalNode, error) {
	var success bool

	connManager, err := connmgr.NewConnManager(
		100, // Lowwater
		400, // HighWater,
		connmgr.WithGracePeriod(time.Minute),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !success {
			connManager.Close()
		}
	}()

	// Create a resource manager configuration
	cfg := rcmgr.PartialLimitConfig{
		System: rcmgr.ResourceLimits{
			StreamsOutbound: rcmgr.DefaultLimit, // Allow unlimited outbound streams
			StreamsInbound:  rcmgr.Unlimited,
			Conns:           4000,
			ConnsInbound:    500,
			ConnsOutbound:   500,
			FD:              10000,
		},
	}
	limiter := rcmgr.NewFixedLimiter(cfg.Build(rcmgr.DefaultLimits.AutoScale()))
	rm, err := rcmgr.NewResourceManager(limiter, rcmgr.WithMetricsDisabled())
	if err != nil {
		return nil, err
	}
	defer func() {
		if !success {
			rm.Close()
		}
	}()

	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	defer func() {
		if !success {
			ds.Close()
		}
	}()

	n := &LocalNode{}

	//var idht *dht.IpfsDHT
	var ddht *dualdht.DHT
	peerChan := make(chan peer.AddrInfo)
	n.Host, err = libp2p.New(
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", p2pPort),
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", p2pPort),
		),
		libp2p.ResourceManager(rm),
		libp2p.Identity(priv),
		libp2p.DefaultSecurity,
		libp2p.DefaultMuxers,
		libp2p.DefaultTransports,
		libp2p.ConnectionManager(connManager),

		libp2p.EnableRelay(),
		libp2p.EnableAutoNATv2(),
		libp2p.EnableHolePunching(holepunch.WithTracer(n)),
		libp2p.NATPortMap(),

		libp2p.FallbackDefaults,
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			var err2 error
			n.DualDht, err2 = newDHT(ctx, h, ds)
			return ddht, err2
		}),

		EnableAutoRelay(peerChan),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !success {
			defer n.Host.Close()
		}
	}()

	AutoRelayFeeder(ctx, n.Host, n.DualDht, nil, peerChan)

	log.Printf("LOCAL Peer ID: %s", n.Host.ID())

	n.PingService = ping.NewPingService(n.Host)

	_, err = waitIdentitySelf(ctx, n.Host, func() error {
		// This connects to public bootstrappers
		for _, addr := range dht.DefaultBootstrapPeers {
			pi, _ := peer.AddrInfoFromP2pAddr(addr)
			err := n.Host.Connect(context.Background(), *pi)
			if err != nil {
				fmt.Println("Boot strap connection failed:", err)
				continue
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	success = true

	return n, nil
}

func waitIdentitySelf(parentCtx context.Context, h host.Host, middleFn func() error) (bool, error) {
	var success bool

	ctx, cancel := context.WithTimeout(parentCtx, time.Second*30)
	defer cancel()

	// Set up a channel to receive notifications about identity updates
	identityChan := make(chan struct{})
	h.SetStreamHandler("/ipfs/id/push/1.0.0", func(stream network.Stream) {
		if hasPublicAddress(h) {
			identityChan <- struct{}{}
		}
	})

	err := middleFn()

	// Wait for identity information to be received
	select {
	case <-identityChan:
		success = true
		fmt.Println("Identity information received for this host")
	case <-ctx.Done():
		fmt.Println("Timeout: Identity information not received within 30 seconds")
	}

	h.RemoveStreamHandler("/ipfs/id/push/1.0.0")

	return success, err
}

func (n *LocalNode) Trace(evt *holepunch.Event) {
	log.Printf("holepunch: evt.Type: %s", evt.Type)
	if evt.Type == holepunch.EndHolePunchEvtT {
		endData, ok := evt.Evt.(*holepunch.EndHolePunchEvt)
		if ok {
			log.Printf("holepunch.Event: %+v; %+v", evt, endData)
		} else {
			log.Printf("UNKNOWN END HOLEPUNCH EVT: %+v", evt)
		}
	}
}

// RecordValidator provides namesys compatible routing record validator
func RecordValidator(ps peerstore.Peerstore) record.Validator {
	return record.NamespacedValidator{
		"pk":   record.PublicKeyValidator{},
		"ipns": ipns.Validator{KeyBook: ps},
	}
}

func newDHT(ctx context.Context, h host.Host, ds datastore.Batching) (*dualdht.DHT, error) {
	dhtOpts := []dht.Option{
		dht.Concurrency(10),
		dht.Mode(dht.ModeAuto),
		dht.Datastore(ds),
		dht.Validator(RecordValidator(h.Peerstore())),
	}

	wanOptions := []dht.Option{
		dht.BootstrapPeers(dht.GetDefaultBootstrapPeerAddrInfos()...),
	}

	return dualdht.New(
		ctx,
		h,
		dualdht.DHTOption(dhtOpts...),
		dualdht.WanDHTOption(wanOptions...),
	)
}

// check if the Host has public address
func hasPublicAddress(host host.Host) bool {
	for _, a := range host.Addrs() {
		str := a.String()
		if !strings.Contains(str, "127.0.0") &&
			!strings.Contains(str, "/ip4/10.") &&
			!strings.Contains(str, "/ip4/172.16.") &&
			!strings.Contains(str, "/ip4/192.168.") {
			return true
		}
	}
	return false
}
