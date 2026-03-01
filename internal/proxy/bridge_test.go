package proxy

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func TestBridgeBidirectional(t *testing.T) {
	client, clientRemote := net.Pipe()
	backend, backendRemote := net.Pipe()

	done := make(chan struct{})
	go func() {
		Bridge(clientRemote, backendRemote)
		close(done)
	}()

	// Client → Backend
	msg := []byte("hello from client")
	if _, err := client.Write(msg); err != nil {
		t.Fatalf("client write: %v", err)
	}

	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(backend, buf); err != nil {
		t.Fatalf("backend read: %v", err)
	}
	if !bytes.Equal(buf, msg) {
		t.Errorf("expected %q, got %q", msg, buf)
	}

	// Backend → Client
	reply := []byte("hello from backend")
	if _, err := backend.Write(reply); err != nil {
		t.Fatalf("backend write: %v", err)
	}

	buf2 := make([]byte, len(reply))
	if _, err := io.ReadFull(client, buf2); err != nil {
		t.Fatalf("client read: %v", err)
	}
	if !bytes.Equal(buf2, reply) {
		t.Errorf("expected %q, got %q", reply, buf2)
	}

	client.Close()
	backend.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Bridge did not return after closing connections")
	}
}

func TestBridgeCloseOnOneSide(t *testing.T) {
	client, clientRemote := net.Pipe()
	backend, backendRemote := net.Pipe()

	done := make(chan struct{})
	go func() {
		Bridge(clientRemote, backendRemote)
		close(done)
	}()

	// Close client side
	client.Close()

	// Backend read should get EOF
	buf := make([]byte, 1)
	_, err := backend.Read(buf)
	if err == nil {
		t.Error("expected error on backend read after client close")
	}

	backend.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Bridge did not return")
	}
}

func TestBridgeLargeData(t *testing.T) {
	client, clientRemote := net.Pipe()
	backend, backendRemote := net.Pipe()

	done := make(chan struct{})
	go func() {
		Bridge(clientRemote, backendRemote)
		close(done)
	}()

	// Send 1MB of data
	data := bytes.Repeat([]byte("x"), 1024*1024)

	go func() {
		_, _ = client.Write(data)
		client.Close()
	}()

	received, err := io.ReadAll(backend)
	if err != nil {
		t.Fatalf("backend read: %v", err)
	}
	if len(received) != len(data) {
		t.Errorf("expected %d bytes, got %d", len(data), len(received))
	}

	backend.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Bridge did not return")
	}
}
