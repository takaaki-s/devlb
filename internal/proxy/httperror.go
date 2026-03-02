package proxy

import (
	"fmt"
	"net"
	"strings"
	"time"
)

var httpMethods = []string{
	"GET ", "POST ", "PUT ", "DELETE ", "PATCH ",
	"HEAD ", "OPTIONS ", "CONNECT ",
}

// IsHTTPRequest checks if the given bytes look like the start of an HTTP request.
func IsHTTPRequest(data []byte) bool {
	s := string(data)
	for _, method := range httpMethods {
		if strings.HasPrefix(s, method) {
			return true
		}
	}
	return false
}

// HTTPErrorResponse generates an HTTP 503 response.
func HTTPErrorResponse(serviceName string, listenPort int) []byte {
	body := fmt.Sprintf("503 Service Unavailable\n\ndevlb: no healthy backend available for port %d\nService: %s\n", listenPort, serviceName)
	resp := fmt.Sprintf("HTTP/1.1 503 Service Unavailable\r\nContent-Type: text/plain\r\nConnection: close\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	return []byte(resp)
}

// PeekAndRespond503 peeks at the connection, and if HTTP, writes a 503 response.
// Returns true if an HTTP response was sent.
func PeekAndRespond503(conn net.Conn, serviceName string, listenPort int) bool {
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	// Reset deadline
	_ = conn.SetReadDeadline(time.Time{})

	if err != nil || n == 0 {
		return false
	}

	if !IsHTTPRequest(buf[:n]) {
		return false
	}

	conn.Write(HTTPErrorResponse(serviceName, listenPort))
	return true
}
