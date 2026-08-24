//go:build !windows

package ipc

import (
	"errors"

	"github.com/Zeliper/zwan/server/config"
)

var errUnsupported = errors.New("ipc: named pipes are Windows-only")

// Serve is unsupported off Windows.
func Serve(h Handler) error { return errUnsupported }

// Call is unsupported off Windows.
func Call(req Request) (Response, error) { return Response{}, errUnsupported }

// Start is unsupported off Windows.
func Start(c config.Config) (Response, error) { return Response{}, errUnsupported }

// Stop is unsupported off Windows.
func Stop() (Response, error) { return Response{}, errUnsupported }

// Status is unsupported off Windows.
func Status() (Response, error) { return Response{}, errUnsupported }
