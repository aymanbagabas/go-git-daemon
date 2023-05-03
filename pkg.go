package daemon

import (
	"context"
	"net"
)

// Serve serves the Git daemon server on the given listener l. Each incoming
// connection is handled in a separate goroutine.
func Serve(l net.Listener) error {
	return DefaultConfig.Serve(l)
}

// ListenAndServe listens on the TCP network address s.Addr and then calls
// Serve to handle requests on incoming connections.
func ListenAndServe() error {
	return DefaultConfig.ListenAndServe()
}

// Close force closes the Git daemon server.
func Close() error {
	return DefaultConfig.Close()
}

// Shutdown gracefully shuts down the Git daemon server without interrupting
// any active connections.
func Shutdown(ctx context.Context) error {
	return DefaultConfig.Shutdown(ctx)
}
