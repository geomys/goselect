// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package envcmd implements the “go env” command.
package envcmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"geomys.org/goup/internal/go/base"
	"geomys.org/goup/internal/go/cfg"
)

func checkEnvWrite(key, val string) error {
	switch key {
	case "GOEXE", "GOGCCFLAGS", "GOHOSTARCH", "GOHOSTOS", "GOMOD", "GOTOOLDIR":
		return fmt.Errorf("%s cannot be modified", key)
	case "GOENV":
		return fmt.Errorf("%s can only be set using the OS environment", key)
	}

	// To catch typos and the like, check that we know the variable.
	if !cfg.CanGetenv(key) {
		return fmt.Errorf("unknown go command variable %s", key)
	}

	// Some variables can only have one of a few valid values. If set to an
	// invalid value, the next cmd/go invocation might fail immediately,
	// even 'go env -w' itself.
	switch key {
	case "GO111MODULE":
		switch val {
		case "", "auto", "on", "off":
		default:
			return fmt.Errorf("invalid %s value %q", key, val)
		}
	case "GOPATH":
		if strings.HasPrefix(val, "~") {
			return fmt.Errorf("GOPATH entry cannot start with shell metacharacter '~': %q", val)
		}
		if !filepath.IsAbs(val) && val != "" {
			return fmt.Errorf("GOPATH entry is relative; must be absolute path: %q", val)
		}
	}

	if !utf8.ValidString(val) {
		return fmt.Errorf("invalid UTF-8 in %s=... value", key)
	}
	if strings.Contains(val, "\x00") {
		return fmt.Errorf("invalid NUL in %s=... value", key)
	}
	if strings.ContainsAny(val, "\v\r\n") {
		return fmt.Errorf("invalid newline in %s=... value", key)
	}
	return nil
}

func updateEnvFile(add map[string]string, del map[string]bool) {
	file, err := cfg.EnvFile()
	if file == "" {
		base.Fatalf("go env: cannot find go env config: %v", err)
	}
	data, err := ioutil.ReadFile(file)
	if err != nil && (!os.IsNotExist(err) || len(add) == 0) {
		base.Fatalf("go env: reading go env config: %v", err)
	}

	lines := strings.SplitAfter(string(data), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	} else {
		lines[len(lines)-1] += "\n"
	}

	// Delete all but last copy of any duplicated variables,
	// since the last copy is the one that takes effect.
	prev := make(map[string]int)
	for l, line := range lines {
		if key := lineToKey(line); key != "" {
			if p, ok := prev[key]; ok {
				lines[p] = ""
			}
			prev[key] = l
		}
	}

	// Add variables (go env -w). Update existing lines in file if present, add to end otherwise.
	for key, val := range add {
		if p, ok := prev[key]; ok {
			lines[p] = key + "=" + val + "\n"
			delete(add, key)
		}
	}
	for key, val := range add {
		lines = append(lines, key+"="+val+"\n")
	}

	// Delete requested variables (go env -u).
	for key := range del {
		if p, ok := prev[key]; ok {
			lines[p] = ""
		}
	}

	// Sort runs of KEY=VALUE lines
	// (that is, blocks of lines where blocks are separated
	// by comments, blank lines, or invalid lines).
	start := 0
	for i := 0; i <= len(lines); i++ {
		if i == len(lines) || lineToKey(lines[i]) == "" {
			sortKeyValues(lines[start:i])
			start = i + 1
		}
	}

	data = []byte(strings.Join(lines, ""))
	err = ioutil.WriteFile(file, data, 0666)
	if err != nil {
		// Try creating directory.
		os.MkdirAll(filepath.Dir(file), 0777)
		err = ioutil.WriteFile(file, data, 0666)
		if err != nil {
			base.Fatalf("go env: writing go env config: %v", err)
		}
	}
}

// lineToKey returns the KEY part of the line KEY=VALUE or else an empty string.
func lineToKey(line string) string {
	i := strings.Index(line, "=")
	if i < 0 || strings.Contains(line[:i], "#") {
		return ""
	}
	return line[:i]
}

// sortKeyValues sorts a sequence of lines by key.
// It differs from sort.Strings in that GO386= sorts after GO=.
func sortKeyValues(lines []string) {
	sort.Slice(lines, func(i, j int) bool {
		return lineToKey(lines[i]) < lineToKey(lines[j])
	})
}
