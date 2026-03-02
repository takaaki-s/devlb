package proxy

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestIsHTTPRequest(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		expect bool
	}{
		{"GET request", []byte("GET / HTTP/1.1\r\n"), true},
		{"POST request", []byte("POST /api HTTP/1.1\r\n"), true},
		{"PUT request", []byte("PUT /data HTTP/1.1\r\n"), true},
		{"DELETE request", []byte("DELETE /item HTTP/1.1\r\n"), true},
		{"PATCH request", []byte("PATCH /update HTTP/1.1\r\n"), true},
		{"HEAD request", []byte("HEAD / HTTP/1.1\r\n"), true},
		{"OPTIONS request", []byte("OPTIONS * HTTP/1.1\r\n"), true},
		{"CONNECT request", []byte("CONNECT host:443 HTTP/1.1\r\n"), true},
		{"binary data", []byte{0x00, 0x01, 0x02, 0x03}, false},
		{"empty", []byte{}, false},
		{"random text", []byte("hello world"), false},
		{"partial GET", []byte("GE"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsHTTPRequest(tt.data)
			if got != tt.expect {
				t.Errorf("IsHTTPRequest(%q) = %v, want %v", tt.data, got, tt.expect)
			}
		})
	}
}

func TestHTTPErrorResponse(t *testing.T) {
	resp := HTTPErrorResponse("api", 8080)
	s := string(resp)

	if !strings.HasPrefix(s, "HTTP/1.1 503 Service Unavailable\r\n") {
		t.Errorf("response should start with HTTP/1.1 503, got: %s", s[:50])
	}
	if !strings.Contains(s, "Content-Type: text/plain") {
		t.Error("response should contain Content-Type: text/plain")
	}
	if !strings.Contains(s, "Connection: close") {
		t.Error("response should contain Connection: close")
	}
	if !strings.Contains(s, "port 8080") {
		t.Error("response body should contain port number")
	}
	if !strings.Contains(s, "api") {
		t.Error("response body should contain service name")
	}
}

func TestPeekAndRespond503_HTTP(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	// Start client-side reader to avoid blocking on net.Pipe
	respCh := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := client.Read(buf)
		respCh <- string(buf[:n])
	}()

	// Send HTTP request from client side
	go func() {
		client.Write([]byte("GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"))
	}()

	result := PeekAndRespond503(server, "api", 8080)
	server.Close()

	if !result {
		t.Error("expected PeekAndRespond503 to detect HTTP and return true")
	}

	resp := <-respCh
	if !strings.Contains(resp, "503") {
		t.Errorf("expected 503 in response, got: %s", resp)
	}
}

func TestPeekAndRespond503_NonHTTP(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	go func() {
		client.Write([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05})
	}()

	result := PeekAndRespond503(server, "api", 8080)
	server.Close()

	if result {
		t.Error("expected PeekAndRespond503 to return false for non-HTTP data")
	}
}

func TestPeekAndRespond503_Timeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	done := make(chan bool, 1)
	go func() {
		result := PeekAndRespond503(server, "api", 8080)
		done <- result
	}()

	// Don't send anything — should timeout
	select {
	case wasHTTP := <-done:
		if wasHTTP {
			t.Error("expected PeekAndRespond503 to return false on timeout")
		}
	case <-time.After(2 * time.Second):
		t.Error("PeekAndRespond503 should not block forever")
	}
}
