package main

import (
	"strings"
	"testing"
)

// `syn --help` is the first thing most people type, and it crashed with a
// stack trace: the flag is taken out of the arguments before the dispatcher
// runs, and the next line indexed into what was left, which was nothing.
func TestBareFlagsDoNotPanic(t *testing.T) {
	for _, args := range [][]string{
		{"--help"}, {"-h"}, {"--version"}, {"-v"}, {"--quiet"}, {"--all"}, {"--json"},
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("syn %s panicked: %v", strings.Join(args, " "), r)
				}
			}()
			if err := run(args); err != nil {
				t.Errorf("syn %s returned an error: %v", strings.Join(args, " "), err)
			}
		}()
	}
}
