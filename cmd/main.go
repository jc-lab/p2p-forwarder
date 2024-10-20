package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/jc-lab/p2p-forwarder/forwarder"
	"github.com/jc-lab/p2p-forwarder/pkg/p2p"
	"github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/things-go/go-socks5"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const help = `
Usage: 1) Run the program in first machine (local) with: ./p2p-forwarder server [flags...] [-allow-proxy]
       2) Then start it in second machine (remote) with: ./p2p-forwarder client [flags...] -d <RemotePeerId> [-proxy-port 8000]
`

type AppMode int

const (
	UnknownMode AppMode = 0
	ServerMode  AppMode = 1
	ClientMode  AppMode = 2
)

type ArrayFlags []string

func (i *ArrayFlags) String() string {
	return fmt.Sprintf("%v", *i)
}

func (i *ArrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

type CommonFlags struct {
	P2pPort      int
	ForwardPorts ArrayFlags
	KeyFile      string
}

type ServerFlags struct {
	*CommonFlags
	AllowProxy bool
}

type ClientFlags struct {
	*CommonFlags
	Ping       bool
	WithPing   bool
	DestPeerId string
	ProxyPort  int
}

func main() {
	var mode AppMode

	var commonFlags CommonFlags
	var serverFlags ServerFlags
	var clientFlags ClientFlags
	serverFlags.CommonFlags = &commonFlags
	clientFlags.CommonFlags = &commonFlags

	serverFlagSet := flag.NewFlagSet("server", flag.ExitOnError)
	serverFlagSet.BoolVar(&serverFlags.AllowProxy, "allow-proxy", false, "allow socks5 proxy server")
	serverFlagSet.Var(&commonFlags.ForwardPorts, "p", "allow forward ports")

	clientFlagSet := flag.NewFlagSet("client", flag.ExitOnError)
	clientFlagSet.IntVar(&clientFlags.ProxyPort, "proxy-port", 0, "socks proxy server listen port")
	clientFlagSet.Var(&commonFlags.ForwardPorts, "p", "forward ports (-p PORT or -p LOCAL_PORT:REMOTE_PORT)")
	clientFlagSet.StringVar(&clientFlags.DestPeerId, "d", "", "destination peer id")
	clientFlagSet.BoolVar(&clientFlags.Ping, "ping", false, "ping test")
	clientFlagSet.BoolVar(&clientFlags.WithPing, "with-ping", false, "ping test")

	flag.Usage = func() {
		fmt.Println(strings.TrimSpace(help))
		flag.PrintDefaults()
	}

	flag.IntVar(&commonFlags.P2pPort, "p2p-port", 12398, "libp2p listen port")
	flag.StringVar(&commonFlags.KeyFile, "key-file", "peer.key", "libp2p peer key file")
	flag.Parse()

	remainingArgs := flag.Args()
	if len(remainingArgs) == 0 {
		flag.Usage()
		return
	}
	switch remainingArgs[0] {
	case "server":
		mode = ServerMode
		serverFlagSet.Parse(remainingArgs[1:])
	case "client":
		mode = ClientMode
		clientFlagSet.Parse(remainingArgs[1:])
	default:
		flag.Usage()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if mode == ServerMode {
		serverApp(ctx, &serverFlags)
	} else {
		clientApp(ctx, &clientFlags)
	}
}

func useKeyFile(keyFile string) (crypto.PrivKey, error) {
	var priv crypto.PrivKey
	raw, err := os.ReadFile(keyFile)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		priv, _, err = crypto.GenerateKeyPair(
			crypto.Ed25519,
			-1,
		)
		if err != nil {
			return nil, err
		}
		keyBytes, _ := crypto.MarshalPrivateKey(priv)
		err = os.WriteFile(keyFile, keyBytes, 0600)
		if err != nil {
			log.Printf("write key file failed: %+v", err)
		}
		return priv, nil
	} else {
		return crypto.UnmarshalPrivateKey(raw)
	}
}

func serverApp(ctx context.Context, flags *ServerFlags) {
	key, err := useKeyFile(flags.KeyFile)
	if err != nil {
		log.Fatal("useKeyFile failed:", err)
	}

	node, err := p2p.NewLocalNode(ctx, key, flags.P2pPort)
	if err != nil {
		log.Fatalf("p2p node create failed: %+v", err)
	}

	if flags.AllowProxy {
		socksServer := socks5.NewServer()

		node.Host.SetStreamHandler(forwarder.SocksProtocol, func(stream network.Stream) {
			forwarder.HandleSocksServer(socksServer, stream)
		})
	}

	for _, text := range flags.ForwardPorts {
		port, err := strconv.Atoi(text)
		if err != nil {
			log.Panicf("parse port(%s) failed: %+v", text, err)
		}
		protocolId := core.ProtocolID(forwarder.TcpProtocol + fmt.Sprintf("/%d", port))
		log.Printf("Listen protocol id: %s", protocolId)
		node.Host.SetStreamHandler(protocolId, func(stream network.Stream) {
			forwarder.HandleTcpServer(port, stream)
		})
	}

	select {}
}

func clientApp(ctx context.Context, flags *ClientFlags) {
	destPeerAddrText := "/p2p-circuit/p2p/" + flags.DestPeerId

	destPeerAddr, err := peer.AddrInfoFromString(destPeerAddrText)
	if err != nil {
		log.Fatalln("failed to decode destination peer id:", err)
	}

	key, err := useKeyFile(flags.KeyFile)
	if err != nil {
		log.Fatalln("useKeyFile failed:", err)
	}
	node, err := p2p.NewLocalNode(ctx, key, flags.P2pPort)
	if err != nil {
		log.Fatalf("p2p node create failed: %+v", err)
	}

	if true {
		p2pAddr, isP2pCircuit := strings.CutPrefix(destPeerAddrText, "/p2p-circuit/")
		if isP2pCircuit {
			var relayedAddresses []multiaddr.Multiaddr
			found, err := node.DualDht.FindPeer(ctx, destPeerAddr.ID)
			if err == nil {
				for _, relayAddr := range found.Addrs {
					tmpAddr, err := peer.AddrInfoFromString(relayAddr.String() + "/" + p2pAddr)
					if err == nil {
						relayedAddresses = append(relayedAddresses, tmpAddr.Addrs...)
					}
				}
			}

			destPeerAddr.Addrs = relayedAddresses
		}
	}

	node.Host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, conn network.Conn) {
			if conn.RemotePeer() == destPeerAddr.ID {
				log.Printf("CONNECTED: %+v", conn)
			}
		},
	})

	err = node.Host.Connect(ctx, *destPeerAddr)
	if err != nil {
		log.Fatalln("failed to connect to remote peer:", err)
	}
	log.Printf("connected")

	conns := node.Host.Network().ConnsToPeer(destPeerAddr.ID)
	for _, c := range conns {
		log.Printf("Connected to peer: %s (%+v)\n", c.RemotePeer(), c.RemoteMultiaddr())
		log.Printf("\t%+v", c.Stat())
	}

	if flags.Ping {
		for {
			doPing(ctx, node, destPeerAddr.ID)
		}
		return
	}
	if flags.WithPing {
		go func() {
			for {
				doPing(ctx, node, destPeerAddr.ID)
			}
		}()
	}

	for _, text := range flags.ForwardPorts {
		tokens := strings.Split(text, ":")
		var err error
		var localPort int
		var peerPort int
		switch len(tokens) {
		case 1:
			peerPort, err = strconv.Atoi(text)
			if err != nil {
				log.Panicf("port[%s] parse failed: %+v", text, err)
			}
			localPort = peerPort
		case 2:
			localPort, err = strconv.Atoi(tokens[0])
			if err != nil {
				log.Panicf("port[%s] parse failed: %+v", tokens[0], err)
			}
			peerPort, err = strconv.Atoi(tokens[1])
			if err != nil {
				log.Panicf("port[%s] parse failed: %+v", tokens[1], err)
			}
		default:
			log.Panicf("port[%s] parse failed: invalid format", text)
		}
		localAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
		listener, err := net.Listen("tcp", localAddr)
		if err != nil {
			log.Panicf("local port [%s] listen failed: %+v", localAddr, err)
		}
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					log.Printf("local port [%s] accept failed: %+v", localAddr, err)
					break
				}
				forwarder.HandleTcp(ctx, node, destPeerAddr.ID, conn, peerPort)
			}
		}()
	}

	if flags.ProxyPort > 0 {
		listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", flags.ProxyPort))
		if err != nil {
			log.Fatalf("socks port (%d) listen failed: %+v", flags.ProxyPort, err)
		}
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("socks accept failed: %+v", err)
				break
			}
			go forwarder.HandleSocks(ctx, node, destPeerAddr.ID, conn)
		}
	}
}

func doPing(ctx context.Context, node *p2p.LocalNode, peerId peer.ID) {
	resultCh := ping.Ping(ctx, node.Host, peerId)
	for {
		result := <-resultCh
		log.Printf("Ping result: %+v", result)
		if result.Error != nil {
			break
		}
		time.Sleep(time.Second)
	}
}
