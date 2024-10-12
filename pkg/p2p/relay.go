package p2p

import (
	"context"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
)

func StaticRelay(staticRelays []string) libp2p.Option {
	var err error
	static := make([]peer.AddrInfo, 0, len(staticRelays))
	for _, s := range staticRelays {
		var addr *peer.AddrInfo
		addr, err = peer.AddrInfoFromString(s)
		if err != nil {
			return func(cfg *libp2p.Config) error {
				return nil
			}
		}
		static = append(static, *addr)
	}
	return libp2p.EnableAutoRelayWithStaticRelays(static)
}

func EnableAutoRelay(peerChan chan peer.AddrInfo) libp2p.Option {
	return libp2p.EnableAutoRelayWithPeerSource(
		func(ctx context.Context, numPeers int) <-chan peer.AddrInfo {
			// TODO(9257): make this code smarter (have a state and actually try to grow the search outward) instead of a long running task just polling our K cluster.
			r := make(chan peer.AddrInfo)
			go func() {
				defer close(r)
				for ; numPeers != 0; numPeers-- {
					select {
					case v, ok := <-peerChan:
						if !ok {
							return
						}
						select {
						case r <- v:
						case <-ctx.Done():
							return
						}
					case <-ctx.Done():
						return
					}
				}
			}()
			return r
		},
		autorelay.WithMinInterval(0),
	)
}
