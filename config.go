package daemon

import (
	"io"
	"net"
	"os/exec"
	"sync"
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

// AccessHook is a git-daemon access hook. It takes a service name, repository
// path, and a client address as arguments.
type AccessHook func(Service, string, string) error

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

	// MaxConnections is the maximum number of simultaneous connections.
	//
	// Zero means no limit.
	MaxConnections int

	// Logger is where to write logs.
	// Use io.Discard to disable logging.
	//
	// Default is os.Stderr.
	Logger io.Writer

	// Verbose is whether or not to enable verbose logging.
	Verbose bool

	// UploadPack is whether or not to enable the upload-pack service.
	UploadPack bool

	// UploadArchive is whether or not to enable the upload-archive service.
	UploadArchive bool

	// ReceivePack is whether or not to enable the receive-pack service.
	ReceivePack bool

	// AccessHook is the access hook.
	// This is called before the service is started every time a client tries
	// to access a repository.
	// If the hook returns an error, the client is denied access and the error
	// is sent to the client.
	//
	// Default is a no-op.
	AccessHook AccessHook

	// CommandFunc is a function hook that gets called on the git
	// *exec.Command. This can be used to modify the command before it is run.
	// For example, to add environment variables.
	CommandFunc func(*exec.Cmd)

	// GitBinPath is the path to the Git binary.
	//
	// Default is "git".
	GitBinPath string

	listener    net.Listener
	connections *connections
	wg          sync.WaitGroup
	mu          sync.Mutex
	done        chan struct{}
}

var (
	// DefaultAddr is the default Git daemon address.
	DefaultAddr = ":9418"

	// DefaultConfig is the default Git daemon configuration.
	DefaultConfig = Config{
		MaxConnections: 32,
		UploadPack:     true,
		UploadArchive:  false,
		ReceivePack:    false,
	}
)
