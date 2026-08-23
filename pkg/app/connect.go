package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/truvity/google-group-sync/gen/directory/v1/directoryv1connect"
	"github.com/truvity/google-group-sync/pkg/directorysvc"
	"github.com/truvity/google-group-sync/pkg/resolver"
)

// startConnect runs the ConnectRPC DirectoryService on its own port until
// ctx is canceled. Unencrypted HTTP/2 (h2c) is enabled so gRPC clients work
// without TLS behind the mesh; Connect/JSON over HTTP/1.1 works too. Returns
// nil when port <= 0 (disabled). Blocking; run in a goroutine.
func startConnect(
	ctx context.Context,
	logger *slog.Logger,
	port int,
	res resolver.GroupLister,
	domains []string,
	adminEmail, probeGroup string,
) error {
	if port <= 0 {
		return nil
	}

	svc := directorysvc.New(logger, res, domains, adminEmail, probeGroup)
	path, handler := directoryv1connect.NewDirectoryServiceHandler(svc)

	mux := http.NewServeMux()
	mux.Handle(path, handler)

	// Serve HTTP/1.1 and unencrypted HTTP/2 (the modern replacement for the
	// deprecated x/net h2c wrapper).
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		// ctx is canceled (that is our trigger); derive a fresh deadline
		// from it so shutdown has time to drain.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logger.InfoContext(ctx, "starting DirectoryService (ConnectRPC)",
		slog.Int("port", port),
		slog.Any("domains", domains),
	)

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("connect server: %w", err)
	}

	return nil
}
