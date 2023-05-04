package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	daemon "github.com/aymanbagabas/go-git-daemon"
	"github.com/spf13/pflag"
)

var (
	logDest    string
	accessHook string
	host       string
	enables    []string
	disables   []string
)

func init() {
	pflag.StringVar(&logDest, "log-destination", "stderr", "log destination")
	pflag.StringArrayVar(&enables, "enable", nil, "enable a service")
	pflag.StringArrayVar(&disables, "disable", nil, "disable a service")
	pflag.StringVar(&host, "host", "localhost", "the server hostname to advertise")

	pflag.StringVar(&daemon.DefaultConfig.Addr, "listen", daemon.DefaultConfig.Addr, "listen on the given address")
	pflag.BoolVar(&daemon.DefaultConfig.StrictPaths, "strict-paths", daemon.DefaultConfig.StrictPaths, "match paths exactly")
	pflag.StringVar(&daemon.DefaultConfig.BasePath, "base-path", daemon.DefaultConfig.BasePath, "base path for all repositories")
	pflag.BoolVar(&daemon.DefaultConfig.ExportAll, "export-all", daemon.DefaultConfig.ExportAll, "export all repositories")
	pflag.IntVar(&daemon.DefaultConfig.InitTimeout, "init-timeout", daemon.DefaultConfig.InitTimeout, "timeout (in seconds) between the moment the connection is established and the client request is received")
	pflag.IntVar(&daemon.DefaultConfig.IdleTimeout, "idle-timeout", daemon.DefaultConfig.IdleTimeout, "timeout (in seconds) between each read/write request")
	pflag.IntVar(&daemon.DefaultConfig.MaxTimeout, "max-timeout", daemon.DefaultConfig.MaxTimeout, "timeout (in seconds) the total time the connection is allowed to be open")
	pflag.IntVar(&daemon.DefaultConfig.MaxConnections, "max-connections", daemon.DefaultConfig.MaxConnections, "maximum number of simultaneous connections")
	pflag.BoolVar(&daemon.DefaultConfig.Verbose, "verbose", daemon.DefaultConfig.Verbose, "enable verbose logging")

	pflag.StringVar(&accessHook, "access-hook", "", "run external command to authorize access")
}

func main() {
	pflag.Parse()

	for _, v := range enables {
		service := daemon.Service(strings.ToLower(v))
		switch service {
		case daemon.UploadPack:
			daemon.DefaultConfig.UploadPack = true
		case daemon.UploadArchive:
			daemon.DefaultConfig.UploadArchive = true
		case daemon.ReceivePack:
			daemon.DefaultConfig.ReceivePack = true
		}
	}

	for _, v := range disables {
		service := daemon.Service(strings.ToLower(v))
		switch service {
		case daemon.UploadPack:
			daemon.DefaultConfig.UploadPack = false
		case daemon.UploadArchive:
			daemon.DefaultConfig.UploadArchive = false
		case daemon.ReceivePack:
			daemon.DefaultConfig.ReceivePack = false
		}
	}

	logDest = strings.ToLower(logDest)
	switch logDest {
	case "stdout":
		daemon.DefaultConfig.Logger = os.Stdout
	case "none":
		daemon.DefaultConfig.Logger = io.Discard
	default:
		if logDest != "stderr" {
			log.Printf("log destination %q not supported, using stderr", logDest)
		}
		daemon.DefaultConfig.Logger = os.Stderr
	}

	if accessHook != "" {
		stat, err := os.Stat(accessHook)
		if err != nil {
			log.Fatal(err)
		}

		if stat.Mode()&0111 == 0 {
			log.Fatalf("access hook %q is not executable", accessHook)
		}

		daemon.DefaultConfig.AccessHook = func(service daemon.Service, path, host, canon, ipAddr, port, remoteAddr string) error {
			var stderr bytes.Buffer
			cmd := exec.Command(accessHook, service.String(), path, host, canon, ipAddr, port, remoteAddr)
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				log.Printf("access hook %q failed: %v", accessHook, err)
				if stderr.Len() > 0 {
					log.Printf("access hook stderr: %s", stderr.String())
				}
				return daemon.ErrAccessDenied
			}

			return nil
		}
	}

	if daemon.DefaultConfig.Verbose {
		log.Printf("Ready to go-rumble")
	}

	if err := daemon.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
