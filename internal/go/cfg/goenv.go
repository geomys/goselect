package cfg

import _ "embed"

// goEnv is the go.env file of the Go release goup is forked from, which
// holds the defaults normally read from $GOROOT/go.env.
//
//go:embed go.env
var goEnv []byte
