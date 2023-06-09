# Git Daemon

<p>
    <a href="https://github.com/aymanbagabas/go-git-daemon/releases"><img src="https://img.shields.io/github/release/aymanbagabas/go-git-daemon.svg" alt="Latest Release"></a>
    <a href="https://pkg.go.dev/github.com/aymanbagabas/go-git-daemon?tab=doc"><img src="https://godoc.org/github.com/golang/gddo?status.svg" alt="Go Docs"></a>
    <a href="https://goreportcard.com/report/github.com/aymanbagabas/go-git-daemon"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/aymanbagabas/go-git-daemon"></a>
</p>

This is a Go implementation of the [Git Transport][git-transport] protocol and `git-daemon` binary. It is meant to be used as a Go library to replace [`git-daemon`](https://git-scm.com/docs/git-daemon), but can also be used as a binary to serve repositories using git transport.

[git-transport]: https://git-scm.com/docs/pack-protocol#_git_transport

## Dependencies

The default _handler_ uses the `git` binary underneath. Specifically, `git-upload-pack`, `git-upload-archive`, and `git-receive-pack`.
The `git` binary is optional if you're using a pure-go implementation, for example [go-git](https://github.com/go-git/go-git), to handle these requests.

## Usage

To import this package as a library

```sh
go get github.com/aymanbagabas/go-git-daemon
```

And then import it in your project

```go
import (
    daemon "github.com/aymanbagabas/go-git-daemon"
)
```

To install the `git-daemon` binary

```sh
go install github.com/aymanbagabas/go-git-daemon/cmd/git-daemon@latest
```

Binary usage

```
Usage of git-daemon:
      --access-hook string       run external command to authorize access
      --base-path string         base path for all repositories
      --disable stringArray      disable a service
      --enable stringArray       enable a service
      --export-all               export all repositories
      --host string              the server hostname to advertise (default "localhost")
      --idle-timeout int         timeout (in seconds) between each read/write request
      --init-timeout int         timeout (in seconds) between the moment the connection is established and the client request is received
      --listen string            listen on the given address (default ":9418")
      --log-destination string   log destination (default "stderr")
      --max-connections int      maximum number of simultaneous connections (default 32)
      --max-timeout int          timeout (in seconds) the total time the connection is allowed to be open
      --strict-paths             match paths exactly
      --timeout int              timeout (in seconds) for specific client sub-requests
      --verbose                  enable verbose logging
```

## Examples

To use this as a server

```go
package main

import (
    "log"

    daemon "github.com/aymanbagabas/go-git-daemon"
)

func main() {
    // Enable upload-archive
    daemon.Enable(daemon.UploadArchiveService)

    // Start server on the default port :9418
    if err := daemon.ListenAndServe(":9418"); err != nil {
        log.Fatal(err)
    }
}
```

Or create a new server instance with custom options and access hook

```go
package main

import (
    "log"

    daemon "github.com/aymanbagabas/go-git-daemon"
)

func accessHook(service Service, path string, host string, canoHost string, ipAdd string, port string, remoteAddr string) error {
    log.Printf("[%s] Requesting access to %s from %s (%s)", service, path, remoteAddr, host)

    // deny any push access except to "public.git"
    if service == daemon.ReceivePackService && path != "/srv/git/public.git" {
        return fmt.Errorf("access denied")
    }

    // deny all push/fetch access to "private.git"
    if path == "/srv/git/private.git" {
        return fmt.Errorf("access denied")
    }

    return nil // allow access
}

func main() {
    logger := log.Default()
    logger.SetPrefix("[git-daemon] ")

    // Create a new server instance
    server := daemon.Server{
        MaxConnections:       0,                                  // unlimited concurrent connections
        Timeout:              3,                                  // 3 seconds timeout
        BasePath:             "/srv/git",                         // base path for all repositories
        ExportAll:            true,                               // export all repositories
        AccessHook:           accessHook,                         // use a custom access hook
        Logger:               logger,                             // use logger
        //Verbose:              true,                             // enable verbose logging
    }

    // Enable all services 💃
    server.Enable(daemon.UploadPackService)
    server.Enable(daemon.UploadArchiveService)
    server.Enable(daemon.ReceivePackService)

    // Start server on the default port (9418)
    if err := server.ListenAndServe("0.0.0.0:9418"); err != nil {
        log.Fatal(err)
    }
}
```

## License

[MIT](./LICENSE)
