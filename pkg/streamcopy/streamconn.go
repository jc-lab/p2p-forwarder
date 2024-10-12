package streamcopy

import (
	"github.com/libp2p/go-libp2p/core/network"
	"net"
	"time"
)

type StreamConn struct {
	stream     network.Stream
	localAddr  StreamAddr
	remoteAddr StreamAddr
}

func NewStreamConn(stream network.Stream, localAddr StreamAddr, remoteAddr StreamAddr) *StreamConn {
	return &StreamConn{
		stream:     stream,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}
}

func (c *StreamConn) Read(b []byte) (n int, err error) {
	return c.stream.Read(b)
}

func (c *StreamConn) Write(b []byte) (n int, err error) {
	return c.stream.Write(b)
}

func (c *StreamConn) Close() error {
	return c.stream.Close()
}

func (c *StreamConn) LocalAddr() net.Addr {
	return &c.localAddr
}

func (c *StreamConn) RemoteAddr() net.Addr {
	return &c.remoteAddr
}

func (c *StreamConn) SetDeadline(t time.Time) error {
	return c.stream.SetDeadline(t)
}

func (c *StreamConn) SetReadDeadline(t time.Time) error {
	return c.stream.SetReadDeadline(t)
}

func (c *StreamConn) SetWriteDeadline(t time.Time) error {
	return c.stream.SetWriteDeadline(t)
}

type StreamAddr struct {
	Text string
}

func (b *StreamAddr) Network() string {
	return "bridge"
}

func (b *StreamAddr) String() string {
	return b.Text
}
