package daemon

import (
	"io"
	"log"
	"net"
	"os/exec"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Service is a Git daemon service.
type Service string

const (
	// UploadPack is the upload-pack service.
	UploadPack Service = "upload-pack"
	// UploadArchive is the upload-archive service.
	UploadArchive Service = "upload-archive"
	// ReceivePack is the receive-pack service.
	ReceivePack Service = "receive-pack"
)

// String returns the string representation of the service.
func (s Service) String() string {
	return string(s)
}

// RequestHandler is a Git daemon request handler. It takes a service name, a
// repository path, and a client connection as
// arguments.
type RequestHandler func(path string, conn net.Conn, cmdFunc func(*exec.Cmd)) error

// AccessHook is a git-daemon access hook. It takes a service name, repository
// path, and a client address as arguments.
type AccessHook func(service Service, path string, host string, canoHost string, ipAdd string, port string, remoteAddr string) error

// Config represents the Git daemon configuration.
type Config struct {
	// Addr is the address the Git daemon listens on.
	//
	// Default is ":9418".
	Addr string

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
	UploadPackHandler RequestHandler

	// UploadArchiveHandler is the upload-archive service handler.
	// If nil, the upload-archive service is disabled.
	UploadArchiveHandler RequestHandler

	// ReceivePackHandler is the receive-pack service handler.
	// If nil, the receive-pack service is disabled.
	ReceivePackHandler RequestHandler

	// AccessHook is the access hook.
	// This is called before the service is started every time a client tries
	// to access a repository.
	// If the hook returns an error, the client is denied access.
	//
	// Default is a no-op.
	AccessHook AccessHook

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

	// DefaultConfig is the default Git daemon configuration.
	DefaultConfig = Config{
		Addr:                 DefaultAddr,
		MaxConnections:       32,
		UploadPackHandler:    DefaultUploadPackHandler,
		UploadArchiveHandler: nil,
		ReceivePackHandler:   nil,
		Logger:               log.Default(),
	}

	// DefaultRequestHandler is the default Git daemon request handler.
	// It uses the git binary to run the requested service.
	// Use cmdFunc to modify the command before it is run (e.g. to add environment variables).
	DefaultRequestHandler = func(service Service, gitBinPath string) RequestHandler {
		return func(path string, conn net.Conn, cmdFunc func(*exec.Cmd)) error {
			cmd := exec.Command(gitBinPath, service.String(), ".") // nolint: gosec
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

			var errg errgroup.Group

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
				return err
			}

			return nil
		}
	}

	// DefaultUploadPackHandler is the default upload-pack service handler.
	DefaultUploadPackHandler = DefaultRequestHandler(UploadPack, GitBinPath)

	// DefaultUploadArchiveHandler is the default upload-archive service handler.
	DefaultUploadArchiveHandler = DefaultRequestHandler(UploadArchive, GitBinPath)

	// DefaultReceivePackHandler is the default receive-pack service handler.
	DefaultReceivePackHandler = DefaultRequestHandler(ReceivePack, GitBinPath)
)
