package forwarder

import (
	"context"
	"fmt"
	"github.com/jc-lab/p2p-forwarder/pkg/p2p"
	"github.com/jc-lab/p2p-forwarder/pkg/streamcopy"
	"github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"log"
	"net"
)

const TcpProtocol = "/port-forward/tcp/1.0.0"

func HandleTcpServer(port int, stream network.Stream) {
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))

	if err != nil {
		log.Printf("Dial(%d) error: %+v", port, err)
		stream.Close()
		return
	}

	streamcopy.BiDirectionCopy(stream, conn.(streamcopy.HalfConn))
}

func HandleTcp(ctx context.Context, node *p2p.LocalNode, peerId peer.ID, conn net.Conn, port int) {
	defer conn.Close()

	protocolId := core.ProtocolID(TcpProtocol + fmt.Sprintf("/%d", port))
	stream, err := node.Host.NewStream(ctx, peerId, protocolId)
	if err != nil {
		log.Printf("open stream failed: %+v", err)
		return
	}
	defer stream.Close()

	streamcopy.BiDirectionCopy(stream, conn.(streamcopy.HalfConn))
}
