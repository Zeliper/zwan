//go:build windows

package ipc

import (
	"encoding/json"
	"net"
	"time"

	"github.com/Microsoft/go-winio"

	"github.com/Zeliper/zwan/server/config"
)

// pipeSDDL grants full access to SYSTEM and Administrators, and generic
// read/write to interactive users, so the user-mode GUI can talk to the SYSTEM
// service. It matches the client engine pipe.
const pipeSDDL = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;IU)"

// Serve listens on the pipe and dispatches requests to h until the listener errors.
func Serve(h Handler) error {
	l, err := winio.ListenPipe(PipeName, &winio.PipeConfig{SecurityDescriptor: pipeSDDL})
	if err != nil {
		return err
	}
	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go serveConn(conn, h)
	}
}

func serveConn(conn net.Conn, h Handler) {
	defer conn.Close()
	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}
	_ = json.NewEncoder(conn).Encode(dispatch(h, req))
}

func dispatch(h Handler, req Request) Response {
	switch req.Op {
	case "start":
		if req.Config == nil {
			return Response{Error: "missing server configuration"}
		}
		if err := h.Start(*req.Config); err != nil {
			st := h.State()
			return Response{Error: err.Error(), State: &st}
		}
		st := h.State()
		return Response{OK: true, State: &st}
	case "stop":
		if err := h.Stop(); err != nil {
			return Response{Error: err.Error()}
		}
		st := h.State()
		return Response{OK: true, State: &st}
	case "status":
		st := h.State()
		return Response{OK: true, State: &st}
	default:
		return Response{Error: "unknown op: " + req.Op}
	}
}

// Call sends one request to the service and returns its response.
func Call(req Request) (Response, error) {
	timeout := 5 * time.Second
	conn, err := winio.DialPipe(PipeName, &timeout)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// Start asks the service to host a network with this configuration.
func Start(c config.Config) (Response, error) { return Call(Request{Op: "start", Config: &c}) }

// Stop asks the service to stop hosting.
func Stop() (Response, error) { return Call(Request{Op: "stop"}) }

// Status asks the service for the current hosting state.
func Status() (Response, error) { return Call(Request{Op: "status"}) }
