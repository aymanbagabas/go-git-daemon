package daemon

import (
	"context"
	"net"
)

// Serve serves the Git daemon server on the given listener l. Each incoming
// connection is handled in a separate goroutine.
func Serve(l net.Listener) error {
	return DefaultServer.Serve(l)
}

// ListenAndServe listens on the TCP network address s.Addr and then calls
// Serve to handle requests on incoming connections.
func ListenAndServe(addr string) error {
	return DefaultServer.ListenAndServe(addr)
}

// Close force closes the Git daemon server.
func Close() error {
	return DefaultServer.Close()
}

// Shutdown gracefully shuts down the Git daemon server without interrupting
// any active connections.
func Shutdown(ctx context.Context) error {
	return DefaultServer.Shutdown(ctx)
}

// HandleUploadPack registers and enables the handler for the UploadPack service.
// If h is nil, the UploadPack service is disabled.
//
// The default handler (DefaultUploadPackHandler) uses the Git command from
// (GitBinPath) and "upload-pack" to handle the service.
//
// To use a different Git binary path, either set GitBinPath or use a custom
// service handler.
func HandleUploadPack(h ServiceHandler) {
	DefaultServer.HandleUploadPack(h)
}

// HandleUploadArchive registers and enables the handler for the UploadArchive service.
// If h is nil, the UploadArchive service is disabled.
//
// The default handler (DefaultUploadArchiveHandler) uses the Git command from
// (GitBinPath) and "upload-archive" to handle the service.
//
// To use a different Git binary path, either set GitBinPath or use a custom
// service handler.
func HandleUploadArchive(h ServiceHandler) {
	DefaultServer.HandleUploadArchive(h)
}

// HandleReceivePack registers and enables the handler for the ReceivePack service.
// If h is nil, the ReceivePack service is disabled.
//
// The default handler (DefaultReceivePackHandler) uses the Git command from
// (GitBinPath) and "receive-pack" to handle the service.
//
// To use a different Git binary path, either set GitBinPath or use a custom
// service handler.
func HandleReceivePack(h ServiceHandler) {
	DefaultServer.HandleReceivePack(h)
}

// Enable enables the given Git transport service.
func Enable(s Service) {
	DefaultServer.Enable(s)
}

// Disable disables the given Git transport service.
func Disable(s Service) {
	DefaultServer.Disable(s)
}
