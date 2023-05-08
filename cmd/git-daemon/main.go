package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"

	daemon "github.com/aymanbagabas/go-git-daemon"
	"github.com/spf13/pflag"
)

var (
	addr       string
	logDest    string
	accessHook string
	enables    []string
	disables   []string
)

func init() {
	pflag.StringVar(&logDest, "log-destination", "stderr", "log destination")
	pflag.StringArrayVar(&enables, "enable", nil, "enable a service")
	pflag.StringArrayVar(&disables, "disable", nil, "disable a service")

	pflag.StringVar(&addr, "listen", daemon.DefaultAddr, "listen on the given address")
	pflag.BoolVar(&daemon.DefaultServer.StrictPaths, "strict-paths", daemon.DefaultServer.StrictPaths, "match paths exactly")
	pflag.StringVar(&daemon.DefaultServer.BasePath, "base-path", daemon.DefaultServer.BasePath, "base path for all repositories")
	pflag.BoolVar(&daemon.DefaultServer.ExportAll, "export-all", daemon.DefaultServer.ExportAll, "export all repositories")
	pflag.IntVar(&daemon.DefaultServer.InitTimeout, "init-timeout", daemon.DefaultServer.InitTimeout, "timeout (in seconds) between the moment the connection is established and the client request is received")
	pflag.IntVar(&daemon.DefaultServer.IdleTimeout, "idle-timeout", daemon.DefaultServer.IdleTimeout, "timeout (in seconds) between each read/write request")
	pflag.IntVar(&daemon.DefaultServer.Timeout, "timeout", daemon.DefaultServer.Timeout, "timeout (in seconds) for specific client sub-requests")
	pflag.IntVar(&daemon.DefaultServer.MaxTimeout, "max-timeout", daemon.DefaultServer.MaxTimeout, "timeout (in seconds) the total time the connection is allowed to be open")
	pflag.IntVar(&daemon.DefaultServer.MaxConnections, "max-connections", daemon.DefaultServer.MaxConnections, "maximum number of simultaneous connections")
	pflag.BoolVar(&daemon.DefaultServer.Verbose, "verbose", daemon.DefaultServer.Verbose, "enable verbose logging")

	pflag.StringVar(&accessHook, "access-hook", "", "run external command to authorize access")
}

func main() {
	pflag.Parse()

	for _, v := range enables {
		service := daemon.Service(strings.ToLower(v))
		daemon.Enable(service)
	}

	for _, v := range disables {
		service := daemon.Service(strings.ToLower(v))
		daemon.Disable(service)
	}

	logDest = strings.ToLower(logDest)
	switch logDest {
	case "stdout":
		daemon.DefaultServer.Logger = log.New(os.Stdout, "", log.LstdFlags)
	case "none":
		daemon.DefaultServer.Logger = nil
	default:
		if logDest != "stderr" {
			log.Printf("log destination %q not supported, using stderr", logDest)
		}
		daemon.DefaultServer.Logger = log.Default()
	}

	if accessHook != "" {
		stat, err := os.Stat(accessHook)
		if err == nil && stat.Mode()&0111 != 0 {
			daemon.DefaultServer.AccessHook = func(service daemon.Service, path, host, canon, ipAddr, port, remoteAddr string) error {
				var stdout bytes.Buffer
				cmd := exec.Command(accessHook, service.String(), path, host, canon, ipAddr, port)
				cmd.Env = os.Environ()
				cmd.Stderr = nil
				cmd.Stdin = nil

				out, err := cmd.StdoutPipe()
				if err != nil {
					return err
				}

				remoteHost, remotePort, err := net.SplitHostPort(remoteAddr)
				if err == nil {
					remoteAddress := remoteHost
					remoteIp := net.ParseIP(remoteAddress)
					if remoteIp != nil && remoteIp.To16() != nil {
						remoteAddress = "[" + remoteAddress + "]"
					}
					cmd.Env = append(cmd.Env, fmt.Sprintf("REMOTE_ADDR=%s", remoteAddress))
					cmd.Env = append(cmd.Env, "REMOTE_PORT="+remotePort)
				}

				if err := cmd.Start(); err != nil {
					return err
				}

				go func() {
					defer out.Close()
					stdout.ReadFrom(out)
				}()

				if err := cmd.Wait(); err != nil {
					log.Printf("access hook %q failed: %v", accessHook, err)
					return err
				}

				return nil
			}
		} else if err != nil {
			log.Printf("access hook %q not found: %s", accessHook, err)
		} else {
			log.Printf("access hook %q not executable: %v", accessHook, stat.Mode().String())
		}
	}

	if daemon.DefaultServer.Verbose {
		log.Printf("Ready to go-rumble")
	}

	if err := daemon.ListenAndServe(addr); err != nil {
		log.Fatal(err)
	}
}
