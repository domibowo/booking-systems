//go:build tools

// Package tools tracks build-time tool dependencies (like the swag CLI used
// by `make swagger`) in go.mod/go.sum, so `go mod tidy` doesn't drop them.
// See https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
package tools

import (
	_ "github.com/swaggo/swag/cmd/swag"
)
