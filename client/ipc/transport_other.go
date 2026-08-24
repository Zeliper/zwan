//go:build !windows

package ipc

import "errors"

var errUnsupported = errors.New("ipc: named pipes are Windows-only")

// Serve is unsupported off Windows.
func Serve(h Handler) error { return errUnsupported }

// Call is unsupported off Windows.
func Call(req Request) (Response, error) { return Response{}, errUnsupported }

// Connect is unsupported off Windows.
func Connect(a ConnectArgs) (Response, error) { return Response{}, errUnsupported }

// Disconnect is unsupported off Windows.
func Disconnect() (Response, error) { return Response{}, errUnsupported }

// Status is unsupported off Windows.
func Status() (Response, error) { return Response{}, errUnsupported }
