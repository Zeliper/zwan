//go:build windows

package ipc

import (
	"encoding/json"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// pipeSDDL grants full access to SYSTEM and Administrators, and generic
// read/write to interactive users, so the user-mode GUI can talk to the SYSTEM
// service.
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
	case "connect":
		if req.Connect == nil {
			return Response{Error: "missing connect args"}
		}
		if err := h.Connect(*req.Connect); err != nil {
			return Response{Error: err.Error()}
		}
		st := h.Status()
		return Response{OK: true, Status: &st}
	case "disconnect":
		if err := h.Disconnect(); err != nil {
			return Response{Error: err.Error()}
		}
		st := h.Status()
		return Response{OK: true, Status: &st}
	case "status":
		st := h.Status()
		return Response{OK: true, Status: &st}
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

// Connect asks the service to join/connect a network.
func Connect(a ConnectArgs) (Response, error) { return Call(Request{Op: "connect", Connect: &a}) }

// Disconnect asks the service to tear the connection down.
func Disconnect() (Response, error) { return Call(Request{Op: "disconnect"}) }

// Status asks the service for the current connection status.
func Status() (Response, error) { return Call(Request{Op: "status"}) }
