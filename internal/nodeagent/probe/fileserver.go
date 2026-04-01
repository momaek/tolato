package probe

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

// ServeTestFile starts an HTTP server that serves a test file of the given size
// for bandwidth testing. It blocks until ctx is cancelled.
func ServeTestFile(ctx context.Context, addr string, sizeMB int, logger *log.Logger) error {
	sizeBytes := int64(sizeMB) * 1024 * 1024

	mux := http.NewServeMux()
	mux.HandleFunc("/testfile", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", sizeBytes))

		buf := make([]byte, 32*1024) // 32KB chunks of zeros
		var written int64
		for written < sizeBytes {
			n := int64(len(buf))
			if remaining := sizeBytes - written; remaining < n {
				n = remaining
			}
			nn, err := w.Write(buf[:n])
			if err != nil {
				return
			}
			written += int64(nn)
		}
	})

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	logger.Printf("serving %dMB test file on %s/testfile", sizeMB, addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("testfile server: %w", err)
	}
	return nil
}
