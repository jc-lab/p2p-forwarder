package p2p

import (
	"context"
	"fmt"
	"github.com/cenkalti/backoff/v4"
	dualdht "github.com/libp2p/go-libp2p-kad-dht/dual"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	circuitv2proto "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/proto"
	"runtime/debug"
	"slices"
	"time"
)

func AutoRelayFeeder(ctx context.Context, h host.Host, dht *dualdht.DHT, trustedPeer []peer.AddrInfo, peerChan chan<- peer.AddrInfo) {
	done := make(chan struct{})

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovering from unexpected error in AutoRelayFeeder:", r)
			debug.PrintStack()
		}
	}()
	go func() {
		defer close(done)

		// Feed peers more often right after the bootstrap, then backoff
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = 5 * time.Second
		bo.Multiplier = 3
		bo.MaxInterval = 1 * time.Hour
		bo.MaxElapsedTime = 0 // never stop
		t := backoff.NewTicker(bo)
		defer t.Stop()
		for {
			select {
			case <-t.C:
			case <-ctx.Done():
				return
			}

			// Always feed trusted IDs (Peering.Peers in the config)
			for _, trustedPeer := range trustedPeer {
				if len(trustedPeer.Addrs) == 0 {
					continue
				}
				select {
				case peerChan <- trustedPeer:
				case <-ctx.Done():
					return
				}
			}

			// Additionally, feed closest peers discovered via DHT
			if dht == nil {
				/* noop due to missing DHT.WAN. happens in some unit tests,
				   not worth fixing as we will refactor this after go-libp2p 0.20 */
				continue
			}

			closestPeers, err := dht.WAN.GetClosestPeers(ctx, h.ID().String())
			if err != nil {
				fallbackAnyNodeAsRelay(ctx, h, peerChan)
				// no-op: usually 'failed to find any peer in table' during startup
				continue
			}
			for _, p := range closestPeers {
				addrs := h.Peerstore().Addrs(p)
				if len(addrs) == 0 {
					continue
				}
				dhtPeer := peer.AddrInfo{ID: p, Addrs: addrs}
				select {
				case peerChan <- dhtPeer:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	//lc.Append(fx.Hook{
	//	OnStop: func(_ context.Context) error {
	//		cancel()
	//		<-done
	//		return nil
	//	},
	//})
}

func fallbackAnyNodeAsRelay(ctx context.Context, h host.Host, peerChan chan<- peer.AddrInfo) {
	time.Sleep(time.Second)

	peerStore := h.Peerstore()
	for _, id := range peerStore.Peers() {
		protos, err := peerStore.GetProtocols(id)
		if err != nil {
			continue
		}

		if !slices.Contains(protos, circuitv2proto.ProtoIDv2Hop) {
			continue
		}

		addr := peerStore.Addrs(id)
		relayPeer := peer.AddrInfo{ID: id, Addrs: addr}

		//log.Printf("FOUND RELAY: %s", relayPeer.String())

		peerChan <- relayPeer
	}
}
