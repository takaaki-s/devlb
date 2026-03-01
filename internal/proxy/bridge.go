package proxy

import (
	"io"
	"net"
	"sync"
)

// Bridge copies data bidirectionally between client and backend.
// It returns when either direction encounters an error or EOF.
func Bridge(client, backend net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	copyAndClose := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if tc, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = tc.CloseWrite()
		} else {
			dst.Close()
		}
	}

	go copyAndClose(backend, client)
	go copyAndClose(client, backend)

	wg.Wait()
}
