package daemon

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"os/exec"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Service is a Git daemon service.
type Service string

const (
	// UploadPackService is the upload-pack service.
	UploadPackService Service = "git-upload-pack"
	// UploadArchiveService is the upload-archive service.
	UploadArchiveService Service = "git-upload-archive"
	// ReceivePackService is the receive-pack service.
	ReceivePackService Service = "git-receive-pack"
)

// String returns the string representation of the service.
func (s Service) String() string {
	return string(s)
}

// Name returns the name of the service.
func (s Service) Name() string {
	return strings.TrimPrefix(s.String(), "git-")
}

// ServiceHandler is a Git transport daemon service handler. It a repository
// path, a client connection as arguments, and an optional command function as
// arguments.
//
// The command function can be used to modify the Git command before it is run
// (for example to add environment variables).
type ServiceHandler func(ctx context.Context, path string, conn net.Conn, cmdFunc func(*exec.Cmd)) error

// AccessHook is a git-daemon access hook. It takes a service name, repository
// path, and a client address as arguments.
type AccessHook func(service Service, path string, host string, canoHost string, ipAdd string, port string, remoteAddr string) error

// Server represents the Git daemon configuration.
type Server struct {
	// StrictPaths match paths exactly (i.e. don’t allow "/foo/repo" when the
	// real path is "/foo/repo.git" or "/foo/repo/.git").
	//
	// Default is false.
	StrictPaths bool

	// BasePath is the base path for all repositories.
	BasePath string

	// ExportAll is whether or not to export all repositories. Allow pulling
	// from all directories that look like Git repositories (have the objects
	// and refs subdirectories), even if they do not have the
	// git-daemon-export-ok file.
	//
	// Default is false.
	ExportAll bool

	// InitTimeout is the timeout (in seconds) between the moment the
	// connection is established and the client request is received
	//
	// Zero means no timeout.
	InitTimeout int

	// IdleTimeout is the timeout (in seconds) between successive client
	// requests and responses.
	//
	// Zero means no timeout.
	IdleTimeout int

	// MaxTimeout is the timeout (in seconds) is the total time the connection
	// is allowed to be open.
	//
	// Zero means no timeout.
	MaxTimeout int

	// Timeout is the timeout (in seconds) for specific client sub-requests.
	// This includes the time it takes for the server to process the
	// sub-request and the time spent waiting for the next client’s request.
	//
	// This timeout is applied to the following sub-requests:
	//  - upload-pack
	//
	// Zero means no timeout.
	Timeout int

	// MaxConnections is the maximum number of simultaneous connections.
	//
	// Zero means no limit.
	MaxConnections int

	// Logger is where to write logs.
	// Use nil to disable logging.
	//
	// Default is log.Defualt.
	Logger *log.Logger

	// Verbose is whether or not to enable verbose logging.
	Verbose bool

	// UploadPackHandler is the upload-pack service handler.
	// If nil, the upload-pack service is disabled.
	UploadPackHandler ServiceHandler

	// UploadArchiveHandler is the upload-archive service handler.
	// If nil, the upload-archive service is disabled.
	UploadArchiveHandler ServiceHandler

	// ReceivePackHandler is the receive-pack service handler.
	// If nil, the receive-pack service is disabled.
	ReceivePackHandler ServiceHandler

	// AccessHook is the access hook.
	// This is called before the service is started every time a client tries
	// to access a repository.
	// If the hook returns an error, the client is denied access.
	//
	// Default is a no-op.
	AccessHook AccessHook

	enabled     map[Service]bool
	listener    net.Listener
	connections *connections
	wg          sync.WaitGroup
	mu          sync.Mutex
	done        chan struct{}
}

var (
	// GitBinPath is the path to the Git binary.
	GitBinPath = "git"

	// DefaultAddr is the default Git daemon address.
	DefaultAddr = ":9418"

	// DefaultServer is the default Git daemon configuration.
	DefaultServer = Server{
		MaxConnections:       32,
		UploadPackHandler:    DefaultUploadPackHandler,
		UploadArchiveHandler: DefaultUploadArchiveHandler,
		ReceivePackHandler:   DefaultReceivePackHandler,
		Logger:               log.Default(),
		enabled:              defaultEnabled,
	}

	defaultEnabled = map[Service]bool{
		UploadPackService:    true,
		UploadArchiveService: false,
		ReceivePackService:   false,
	}

	// DefaultServiceHandler is the default Git transport service handler.
	// It uses the git binary to run the requested service.
	//
	// Use this to implement custom service handlers that encapsulates the Git
	// binary. For example:
	//
	//  myHandler := func(service daemon.Service, gitBinPath string) daemon.ServiceHandler {
	//      return func(path string, conn net.Conn, cmdFunc func(*exec.Cmd)) error {
	//          customFunc := func(cmd *exec.Cmd) {
	//          cmd.Env = append(cmd.Env, "CUSTOM_ENV_VAR=foo")
	//          if cmdFunc != nil {
	//            cmdFunc(cmd)
	//          }
	//
	//          return daemon.DefaultServiceHandler(service, gitBinPath)(path, conn, customFunc)
	//      }
	//  }
	//
	//  daemon.HandleUploadPack(myHandler(daemon.UploadPack, "/path/to/my/git"))
	//  daemon.DefaultServer.ReceivePackHandler = myHandler(daemon.ReceivePack, "/path/to/my/git")
	//
	DefaultServiceHandler = func(service Service, gitBinPath string) ServiceHandler {
		return func(ctx context.Context, path string, conn net.Conn, cmdFunc func(*exec.Cmd)) error {
			cmd := exec.CommandContext(ctx, gitBinPath, service.Name(), ".") // nolint: gosec
			cmd.Dir = path
			if cmdFunc != nil {
				cmdFunc(cmd)
			}

			stdin, err := cmd.StdinPipe()
			if err != nil {
				return err
			}

			stdout, err := cmd.StdoutPipe()
			if err != nil {
				return err
			}

			stderr, err := cmd.StderrPipe()
			if err != nil {
				return err
			}

			if err := cmd.Start(); err != nil {
				return err
			}

			errg, ctx := errgroup.WithContext(ctx)

			// stdin
			errg.Go(func() error {
				defer stdin.Close() // nolint: errcheck
				_, err := io.Copy(stdin, conn)
				return err
			})

			// stdout
			errg.Go(func() error {
				_, err := io.Copy(conn, stdout)
				return err
			})

			// stderr
			errg.Go(func() error {
				_, err := io.Copy(conn, stderr)
				return err
			})

			if err := errg.Wait(); err != nil {
				return errors.Join(err, cmd.Wait())
			}

			return cmd.Wait()
		}
	}

	// DefaultUploadPackHandler is the default upload-pack service handler.
	DefaultUploadPackHandler = DefaultServiceHandler(UploadPackService, GitBinPath)

	// DefaultUploadArchiveHandler is the default upload-archive service handler.
	DefaultUploadArchiveHandler = DefaultServiceHandler(UploadArchiveService, GitBinPath)

	// DefaultReceivePackHandler is the default receive-pack service handler.
	DefaultReceivePackHandler = DefaultServiceHandler(ReceivePackService, GitBinPath)
)

// HandleUploadPack sets the upload-pack service handler.
func (s *Server) HandleUploadPack(handler ServiceHandler) {
	s.UploadPackHandler = handler
}

// HandleUploadArchive sets the upload-archive service handler.
func (s *Server) HandleUploadArchive(handler ServiceHandler) {
	s.UploadArchiveHandler = handler
}

// HandleReceivePack sets the receive-pack service handler.
func (s *Server) HandleReceivePack(handler ServiceHandler) {
	s.ReceivePackHandler = handler
}

// Enable enables the given service.
func (s *Server) Enable(service Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled[service] = true
}

// Disable disables the given service.
func (s *Server) Disable(service Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled[service] = false
}
