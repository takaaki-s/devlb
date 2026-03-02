package proxy

import (
	"net"
	"sync/atomic"
)

// CountingConn wraps a net.Conn and counts bytes read and written.
type CountingConn struct {
	net.Conn
	bytesRead    atomic.Int64
	bytesWritten atomic.Int64
}

// NewCountingConn wraps a net.Conn with byte counting.
func NewCountingConn(conn net.Conn) *CountingConn {
	return &CountingConn{Conn: conn}
}

func (c *CountingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		c.bytesRead.Add(int64(n))
	}
	return n, err
}

func (c *CountingConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 {
		c.bytesWritten.Add(int64(n))
	}
	return n, err
}

// BytesRead returns total bytes read from this connection.
func (c *CountingConn) BytesRead() int64 {
	return c.bytesRead.Load()
}

// BytesWritten returns total bytes written to this connection.
func (c *CountingConn) BytesWritten() int64 {
	return c.bytesWritten.Load()
}
