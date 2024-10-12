# p2p-forwarder

p2p socks proxy

# Usage

### Common

```text
$ p2p-forwarder -help
Usage: 1) Run the program in first machine (local) with: ./p2p-forwarder server [flags...] [-allow-proxy]
       2) Then start it in second machine (remote) with: ./p2p-forwarder client [flags...] -d <RemotePeerId> [-proxy-port 8000]
  -key-file string
        libp2p peer key file (default "peer.key")
  -p2p-port int
        libp2p listen port (default 12398)
```

### Server

```text
$ p2p-forwarder server
Usage of server:
  -allow-proxy
        allow socks5 proxy server
  -p value
        allow forward ports
```

### Client

```text
$ p2p-forwarder client
Usage of client:
  -d string
        destination peer id
  -p value
        forward ports (-p PORT or -p REMOTE_PORT:LOCAL_PORT)
  -ping
        ping test
  -proxy-port int
        socks proxy server listen port
  -with-ping
        ping test
```

# License

[Apache 2.0](./LICENSE)
