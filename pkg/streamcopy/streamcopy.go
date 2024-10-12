package streamcopy

import (
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/pkg/errors"
	"io"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type HalfConn interface {
	io.Closer
	io.Reader
	io.Writer
	SetReadDeadline(t time.Time) error
	CloseWrite() error
	CloseRead() error
}

func IsDeadlineErr(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), "deadline") {
		return true
	}
	return errors.Is(err, os.ErrDeadlineExceeded)
}

func Copy(prunning *int32, src HalfConn, dst HalfConn) error {
	var buf [65536]byte
	var rerr error
	var werr error

	for atomic.LoadInt32(prunning) == 1 {
		var n int
		src.SetReadDeadline(time.Now().Add(time.Second * 3))
		n, rerr = src.Read(buf[:])
		if n > 0 {
			_, werr = dst.Write(buf[:n])
		}
		if rerr != nil && IsDeadlineErr(rerr) {
			rerr = nil
		}
		if rerr != nil || werr != nil {
			break
		}
	}

	src.CloseRead()
	dst.CloseWrite()

	if werr != nil && werr != io.EOF {
		return werr
	}
	if rerr != nil && rerr != io.EOF {
		return rerr
	}
	return nil
}

func BiDirectionCopy(stream network.Stream, conn HalfConn) {
	var running int32 = 1
	exitCh := make(chan error, 1)

	go func() {
		exitCh <- Copy(&running, conn, stream)
		atomic.StoreInt32(&running, 0)
	}()
	if err := Copy(&running, stream, conn); err != nil {
		log.Println("connToStreamCopy error: ", err)
	}
	atomic.StoreInt32(&running, 0)
	err := <-exitCh
	if err != nil && err != io.EOF {
		log.Println("streamToConnCopy error: ", err)
	}
	stream.Close()
	conn.Close()
	log.Println("streamBiDirectionCopy done")
}
