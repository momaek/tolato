package probe

import (
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
)

// ServeTestFile starts an HTTP server that serves a random file of the given size (MB).
func ServeTestFile(port int, sizeMB int) error {
	// Generate random data in memory
	data := make([]byte, sizeMB*1024*1024)
	rand.Read(data)

	http.HandleFunc("/testfile", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.Write(data)
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("[probe] serving %dMB test file on %s/testfile", sizeMB, addr)
	return http.ListenAndServe(addr, nil)
}
