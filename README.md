# Git Daemon

This is a Go implementation of the [Git Transport](https://git-scm.com/docs/pack-protocol#_git_transport) protocol and `git-daemon` binary. It is meant to be used as a Go library to replace `git-daemon` but you can also use [`cmd/git-daemon`](./cmd/git-daemon) as a binary to host repositories.

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

To use this as a library, simply
