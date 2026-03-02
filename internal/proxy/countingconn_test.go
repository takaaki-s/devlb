package proxy

import (
	"net"
	"testing"
)

func TestCountingConnRead(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	cc := NewCountingConn(client)
	defer cc.Close()

	// Write data from server side
	go func() {
		server.Write([]byte("hello"))
		server.Close()
	}()

	buf := make([]byte, 10)
	n, _ := cc.Read(buf)
	if n != 5 {
		t.Errorf("Read returned %d bytes, want 5", n)
	}
	if cc.BytesRead() != 5 {
		t.Errorf("BytesRead = %d, want 5", cc.BytesRead())
	}
	if cc.BytesWritten() != 0 {
		t.Errorf("BytesWritten = %d, want 0", cc.BytesWritten())
	}
}

func TestCountingConnWrite(t *testing.T) {
	server, client := net.Pipe()
	cc := NewCountingConn(client)

	// Read from server side in background
	go func() {
		buf := make([]byte, 100)
		server.Read(buf)
		server.Close()
	}()

	n, err := cc.Write([]byte("world"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("Write returned %d, want 5", n)
	}
	if cc.BytesWritten() != 5 {
		t.Errorf("BytesWritten = %d, want 5", cc.BytesWritten())
	}
	if cc.BytesRead() != 0 {
		t.Errorf("BytesRead = %d, want 0", cc.BytesRead())
	}
	cc.Close()
}

func TestCountingConnMultipleOps(t *testing.T) {
	server, client := net.Pipe()
	cc := NewCountingConn(client)

	go func() {
		server.Write([]byte("abc"))
		buf := make([]byte, 100)
		server.Read(buf)
		server.Write([]byte("def"))
		server.Close()
	}()

	buf := make([]byte, 10)
	cc.Read(buf)      // read "abc" (3 bytes)
	cc.Write([]byte("12345")) // write 5 bytes
	cc.Read(buf)      // read "def" (3 bytes)

	if cc.BytesRead() != 6 {
		t.Errorf("BytesRead = %d, want 6", cc.BytesRead())
	}
	if cc.BytesWritten() != 5 {
		t.Errorf("BytesWritten = %d, want 5", cc.BytesWritten())
	}
	cc.Close()
}
