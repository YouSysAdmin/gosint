// Gosint, Copyright (c) 2026 YouSysAdmin

// Package pkg owns build-time identity (binary name + version string).
// Version is a var so the build can override it via:
//
//	-ldflags "-X github.com/yousysadmin/gosint/pkg.Version=$(git describe --tags --always --dirty)"
package pkg

// AppName is the binary's own name, used wherever the product
// identifies itself to something outside the process.
const AppName = "gosint"

// Version is "devel" in a plain `go build` and the git description in
// a release.
var Version = "devel"
