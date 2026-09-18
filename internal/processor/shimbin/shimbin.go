// Package shimbin embeds the compiled shim binary (cmd/shim) into the rageta
// binary itself, so it can be baked directly into a task's container
// filesystem via LLB without depending on a separate image or on the local
// filesystem of whatever machine happens to be running rageta.
//
// The embedded "shim" file is a placeholder until `make build` overwrites it
// with the real cmd/shim binary compiled for the target platform, before
// compiling cmd/cli.
package shimbin

import _ "embed"

//go:embed shim
var Binary []byte
