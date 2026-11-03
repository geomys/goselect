package main

import (
	"context"

	"geomys.org/goup/internal/go/base"
)

// init populates base.Go with the commands and subcommands of the upstream go
// command, which lookupCmd needs to find the -C flag. Only their names matter,
// as goup never runs them.
func init() {
	cmd := func(name string, subcommands ...*base.Command) *base.Command {
		c := &base.Command{UsageLine: "go " + name, Commands: subcommands}
		if len(subcommands) == 0 {
			c.Run = func(context.Context, *base.Command, []string) {}
		}
		return c
	}
	base.Go.Commands = []*base.Command{
		cmd("bug"),
		cmd("build"),
		cmd("clean"),
		cmd("doc"),
		cmd("env"),
		cmd("fix"),
		cmd("fmt"),
		cmd("generate"),
		cmd("get"),
		cmd("install"),
		cmd("list"),
		cmd("mod",
			cmd("mod download"),
			cmd("mod edit"),
			cmd("mod graph"),
			cmd("mod init"),
			cmd("mod tidy"),
			cmd("mod vendor"),
			cmd("mod verify"),
			cmd("mod why"),
		),
		cmd("run"),
		cmd("telemetry"),
		cmd("test"),
		cmd("tool"),
		cmd("version"),
		cmd("vet"),
		cmd("work",
			cmd("work edit"),
			cmd("work init"),
			cmd("work sync"),
			cmd("work use"),
			cmd("work vendor"),
		),
	}
}
