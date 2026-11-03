# goup

goup is a `go` command that doesn't include a Go toolchain. Instead, it
selects, downloads, and runs the toolchain based on the `GOTOOLCHAIN`
environment variable and the `go.mod` file, like the go command of a Go
installation does when [switching toolchains](https://go.dev/doc/toolchain).

It can be installed as the sole system `go` command, letting toolchains be
selected and fetched automatically based on the environment and module.

goup itself doesn't require updating as often as a full Go installation,
while updating the minimum toolchain version system-wide is as simple as

    go env -w GOTOOLCHAIN=go1.27.3+auto

It's implemented as a stripped down fork of upstream's cmd/go, retaining only
the toolchain selection and module download code. There are just ~300 lines of
new code.

## Installing

You can install `geomys.org/goup/cmd/go` with Go, but that somewhat defeats
the point: goup is meant to be the "root" `go` command of the system.

Pre-built binaries for Linux, macOS, Windows, and FreeBSD are available on the
[releases page](https://github.com/geomys/goup/releases). Extract the `go`
binary into a directory in `$PATH`, ahead of any other Go installation, for
example

    tar -xzf goup-*-linux-amd64.tar.gz -C /usr/local/bin --strip-components=1 goup/go

On macOS and Linux, goup can also be installed with Homebrew, in place of
the `go` formula.

    brew install geomys/tap/goup

## Selecting toolchains

goup selects toolchains with [the same logic as the upstream Go
command](https://go.dev/doc/toolchain#select).

In a module or workspace, that means it uses the toolchain from the `toolchain`
line or the `go` line of the `go.mod` or `go.work` file.

A minimum toolchain can be set with `GOTOOLCHAIN`, for example with

    go env -w GOTOOLCHAIN=go1.27.3+auto

while `GOTOOLCHAIN=go1.27.3` runs exactly that toolchain.

Toolchains are downloaded into the module cache, and are always verified against
the checksum database.

Outside a module, if `GOTOOLCHAIN` doesn't indicate a version, goup runs the
latest patch release of the previous major release.

A `go` line older than Go 1.21 is an error: such go lines were language
versions, and those toolchains can't switch toolchains themselves. This can be
worked around by setting a minimum toolchain with `GOTOOLCHAIN`.

`GOTOOLCHAIN=local` is not supported.
