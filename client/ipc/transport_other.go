//go:build !windows

package ipc

import (
	"errors"

	"github.com/Zeliper/zwan/client/profile"
)

var errUnsupported = errors.New("ipc: named pipes are Windows-only")

// Serve is unsupported off Windows.
func Serve(h Handler) error { return errUnsupported }

// Call is unsupported off Windows.
func Call(req Request) (Response, error) { return Response{}, errUnsupported }

// Connect is unsupported off Windows.
func Connect(n profile.Network) (Response, error) { return Response{}, errUnsupported }

// Disconnect is unsupported off Windows.
func Disconnect(alias string) (Response, error) { return Response{}, errUnsupported }

// Forget is unsupported off Windows.
func Forget(alias string) (Response, error) { return Response{}, errUnsupported }

// Status is unsupported off Windows.
func Status() (Response, error) { return Response{}, errUnsupported }
