package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/git-lfs/pktline"
	"golang.org/x/sync/errgroup"
)

var (
	// ErrServerClosed indicates that the server has been closed.
	ErrServerClosed = errors.New("server closed")

	// ErrTimeout is returned when the maximum read timeout is exceeded.
	ErrTimeout = errors.New("I/O timeout reached")

	// ErrInvalidRequest represents an invalid request.
	ErrInvalidRequest = errors.New("invalid request")

	// ErrAccessDenied represents an access denied error.
	ErrAccessDenied = errors.New("access denied")

	// ErrNotFound represents a repository not found error.
	ErrNotFound = errors.New("repository not found")

	// ErrSystemMalfunction represents a system malfunction error.
	ErrSystemMalfunction = errors.New("something went wrong")
)

var (
	defaultAccessHook  = func(Service, string, string, string, string, string, string) error { return nil }
	defaultCommandFunc = func(*exec.Cmd) {}
)

// init sets the default values for the server configuration.
func (s *Config) init() {
	if s.Addr == "" {
		s.Addr = DefaultAddr
	}

	if s.Logger == nil {
		s.Logger = os.Stderr
	}

	if s.GitBinPath == "" {
		s.GitBinPath = "git"
	}

	if s.done == nil {
		s.done = make(chan struct{})
	}

	if s.connections == nil || s.connections.m == nil {
		s.connections = &connections{
			m: make(map[net.Conn]struct{}),
		}
	}

	if s.AccessHook == nil {
		s.AccessHook = defaultAccessHook
	}

	if s.CommandFunc == nil {
		s.CommandFunc = defaultCommandFunc
	}

	if s.InitTimeout < 0 {
		s.InitTimeout = 0
	}

	if s.IdleTimeout < 0 {
		s.IdleTimeout = 0
	}
}

// ListenAndServe listens on the TCP network address s.Addr and then calls
// Serve to handle requests on incoming connections.
func (s *Config) ListenAndServe() error {
	s.init()
	l, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}

	return s.Serve(l)
}

// Serve serves the Git daemon server on the given listener l. Each incoming
// connection is handled in a separate goroutine.
func (s *Config) Serve(l net.Listener) error {
	if l == nil {
		return fmt.Errorf("git-daemon: listener is nilt: %w", ErrSystemMalfunction)
	}

	s.init()
	s.listener = l

	// Clean base path.
	s.BasePath = filepath.Clean(s.BasePath)

	s.debugf("listening on %s", s.Addr)

	s.debugf("serving repositories from %q", s.BasePath)

	var tempDelay time.Duration // how long to sleep on accept failure

	for {
		select {
		case <-s.done:
			return ErrServerClosed
		default:
			conn, err := s.listener.Accept()
			if err != nil && errors.Is(err, net.ErrClosed) {
				return ErrServerClosed
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				s.logf("Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			if err != nil {
				s.logf("failed to accept connection: %v", err)
				continue
			}

			// Check if we reached the maximum number of simultaneous connections.
			if s.MaxConnections > 0 && s.connections.Size() >= s.MaxConnections {
				msg := "too many connections"
				s.debugf("%s, rejecting %s", msg, conn.RemoteAddr())
				if err := s.packetWriteMsg(conn, msg); err != nil {
					s.logf("failed to write message: %v", err)
				}
				conn.Close() // nolint: errcheck
				continue
			}

			tempDelay = 0
			s.wg.Add(1)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sconn := &serverConn{
				Conn:          conn,
				initTimeout:   time.Duration(s.InitTimeout) * time.Second,
				idleTimeout:   time.Duration(s.IdleTimeout) * time.Second,
				closeCanceler: cancel,
			}
			if s.MaxTimeout > 0 {
				sconn.maxDeadline = time.Now().Add(time.Duration(s.MaxTimeout) * time.Second)
			}

			s.connections.Add(sconn)
			go s.handleConn(ctx, conn)
		}
	}
}

func (s *Config) handleConn(ctx context.Context, c net.Conn) {
	defer s.wg.Done()
	defer s.connections.Close(c) // nolint: errcheck

	pktc := make(chan []byte, 1)
	errc := make(chan error, 1)
	line := pktline.NewPktline(c, c)
	go func() {
		pkt, err := line.ReadPacket()
		if err != nil {
			errc <- err
			return
		}
		pktc <- pkt
	}()

	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != nil {
			s.debugf("context error: %v", err)
		}
		s.fatal(c, ErrTimeout) // nolint: errcheck
		return
	case err := <-errc:
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				s.debugf("timeout reading pktline from %s", c.RemoteAddr())
				s.fatal(c, ErrTimeout) // nolint: errcheck
			} else {
				s.debugf("error scanning pktline: %v", err)
				s.fatal(c, ErrInvalidRequest) // nolint: errcheck
			}
			return
		}
	case pkt := <-pktc:
		split := bytes.SplitN(pkt, []byte{' '}, 2)
		if len(split) != 2 {
			s.fatal(c, ErrInvalidRequest) // nolint: errcheck
			return
		}

		service := Service(bytes.TrimPrefix(split[0], []byte("git-")))
		switch service {
		case UploadPack:
			if !s.UploadPack {
				s.debugf("upload-pack service not enabled for %s", c.RemoteAddr())
				s.fatal(c, ErrAccessDenied) // nolint: errcheck
				return
			}
		case UploadArchive:
			if !s.UploadArchive {
				s.debugf("upload-archive service not enabled for %s", c.RemoteAddr())
				s.fatal(c, ErrAccessDenied) // nolint: errcheck
				return
			}
		case ReceivePack:
			if !s.ReceivePack {
				s.debugf("receive-pack service not enabled for %s", c.RemoteAddr())
				s.fatal(c, ErrAccessDenied) // nolint: errcheck
				return
			}
		default:
			s.fatal(c, ErrInvalidRequest) // nolint: errcheck
			return
		}

		opts := bytes.Split(split[1], []byte{'\x00'})
		if len(opts) == 0 {
			s.fatal(c, ErrInvalidRequest) // nolint: errcheck
			return
		}

		var host string
		var version int

		for _, o := range opts {
			opt := string(o)
			if opt == "" {
				continue
			}

			if s.Verbose {
				s.debugf("received option %q", opt)
			}

			switch {
			case strings.HasPrefix(opt, "host="):
				host = strings.TrimPrefix(opt, "host=")
			case strings.HasPrefix(opt, "version="):
				version, _ = strconv.Atoi(strings.TrimPrefix(opt, "version="))
			}
		}

		if s.Verbose {
			s.debugf("protocol version %d", version)
		}

		var (
			hostname      string
			canonHostname string
			ipAddr        string
			port          string
		)

		if host != "" {
			url, err := url.Parse(host)
			if err == nil {
				// FIXME: this is not correct, we should use the canonical hostname
				hostname = url.Hostname()
				canonHostname = url.Hostname()
			}
		}

		tcpAddr, err := net.ResolveTCPAddr("tcp", host)
		if err == nil {
			ipAddr = tcpAddr.IP.String()
			port = strconv.Itoa(tcpAddr.Port)
		}

		remoteAddr := c.RemoteAddr().String()
		actualPath := string(opts[0])
		actualPath = filepath.Join(s.BasePath, actualPath)
		actualPath = filepath.Clean(actualPath)

		// validate path
		path := s.validatePath(actualPath)

		s.debugf("connect %s %s %s", remoteAddr, service, actualPath)
		defer s.debugf("disconnect %s %s %s", remoteAddr, service, actualPath)

		if path == "" {
			s.fatal(c, fmt.Errorf("%w: %s", ErrNotFound, actualPath)) // nolint: errcheck
			return
		}

		if !s.ExportAll && !isExportOk(path) {
			s.fatal(c, ErrAccessDenied) // nolint: errcheck
			return
		}

		if s.AccessHook != nil {
			if err := s.AccessHook(service, path, hostname, canonHostname, ipAddr, port, remoteAddr); err != nil {
				s.fatal(c, err) // nolint: errcheck
				return
			}
		}

		cmd := exec.Command(s.GitBinPath, service.String(), path) // nolint: gosec
		cmd.Dir = s.BasePath
		if s.CommandFunc != nil {
			s.CommandFunc(cmd)
		}

		stdin, err := cmd.StdinPipe()
		if err != nil {
			s.logf("failed to create stdin pipe: %v", err)
			s.fatal(c, ErrAccessDenied) // nolint: errcheck
			return
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			s.logf("failed to create stdout pipe: %v", err)
			s.fatal(c, ErrAccessDenied) // nolint: errcheck
			return
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			s.logf("failed to create stderr pipe: %v", err)
			s.fatal(c, ErrAccessDenied) // nolint: errcheck
			return
		}

		if err := cmd.Start(); err != nil {
			s.logf("failed to start git command: %v", err)
			s.fatal(c, ErrAccessDenied) // nolint: errcheck
			return
		}

		var errg errgroup.Group

		// stdin
		errg.Go(func() error {
			defer stdin.Close() // nolint: errcheck

			_, err := io.Copy(stdin, c)
			return err
		})

		// stdout
		errg.Go(func() error {
			_, err := io.Copy(c, stdout)
			return err
		})

		// stderr
		errg.Go(func() error {
			_, err := io.Copy(c, stderr)
			return err
		})

		if err := errg.Wait(); err != nil {
			s.logf("while running git command: %v", err)
			s.fatal(c, ErrAccessDenied) // nolint: errcheck
			return
		}
	}
}

// validatePath checks if the path is valid and if it's a git repository.
// It returns the valid path or empty string if the path is invalid.
func (s *Config) validatePath(path string) string {
	check := func(path string) bool {
		_, err := os.Stat(path)
		if err != nil {
			s.debugf("path %q does not exist", path)
			if !os.IsNotExist(err) {
				s.logf("failed to stat path: %v", err)
			}
			if s.StrictPaths {
				s.debugf("strict paths enabled, returning empty path")
				return false
			}
		} else {
			s.debugf("path %q exists", path)
			if isGitDir(path) {
				s.debugf("path %q is a git repository", path)
				return true
			}
		}

		return false
	}

	for _, suf := range []string{
		"",
		".git",
		"/.git",
		".git/.git",
	} {
		suf = strings.ReplaceAll(suf, "/", string(os.PathSeparator))
		_path := path + suf
		if check(_path) {
			return _path
		}
	}

	return ""
}

// Close force closes the Git daemon server.
func (s *Config) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.connections.CloseAll()
	if s.listener != nil {
		return s.listener.Close()
	}

	return nil
}

// Shutdown gracefully shuts down the Git daemon server without interrupting
// any active connections.
func (s *Config) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	err := s.listener.Close()
	s.mu.Unlock()

	done := make(chan struct{}, 1)
	go func() {
		s.wg.Wait()
		done <- struct{}{}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return err
	}
}

func (s *Config) packetWriteMsg(c net.Conn, msg string) error {
	pkt := pktline.NewPktline(c, c)
	if err := pkt.WritePacketText(msg); err != nil {
		return fmt.Errorf("git-daemon: failed to write message: %w", err)
	}

	return pkt.WriteFlush()
}

func (s *Config) packetWriteErr(c net.Conn, err error) error {
	return s.packetWriteMsg(c, fmt.Sprintf("ERR %s", err)) // nolint: errcheck
}

func (s *Config) fatal(c net.Conn, err error) error {
	s.packetWriteErr(c, err) // nolint: errcheck

	return s.connections.Close(c)
}

func (s *Config) logf(format string, args ...interface{}) {
	format = "git-daemon: " + format + "\n"
	fmt.Fprintf(s.Logger, format, args...)
}

func (s *Config) debugf(format string, args ...interface{}) {
	if s.Verbose {
		s.logf(format, args...)
	}
}
