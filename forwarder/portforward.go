package forwarder

import (
	"context"
	"github.com/jc-lab/p2p-forwarder/pkg/p2p"
	"github.com/jc-lab/p2p-forwarder/pkg/streamcopy"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/things-go/go-socks5"
	"log"
	"net"
)

const SocksProtocol = "/port-forward/socks/1.0.0"

func HandleSocksServer(server *socks5.Server, stream network.Stream) {
	err := server.ServeConn(streamcopy.NewStreamConn(
		stream,
		streamcopy.StreamAddr{
			Text: stream.Conn().LocalPeer().String(),
		},
		streamcopy.StreamAddr{
			Text: stream.Conn().RemotePeer().String(),
		},
	))
	log.Printf("HandleSocksServer error: %+v", err)
}

func HandleSocks(ctx context.Context, node *p2p.LocalNode, peerId peer.ID, conn net.Conn) {
	defer conn.Close()

	stream, err := node.Host.NewStream(ctx, peerId, SocksProtocol)
	if err != nil {
		log.Printf("open stream failed: %+v", err)
		return
	}
	defer stream.Close()

	streamcopy.BiDirectionCopy(stream, conn.(streamcopy.HalfConn))
}
