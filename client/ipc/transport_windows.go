//go:build windows

package ipc

import (
	"encoding/json"
	"net"
	"time"

	"github.com/Microsoft/go-winio"

	"github.com/Zeliper/zwan/client/profile"
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
	var err error
	switch req.Op {
	case "connect":
		if req.Network == nil {
			return Response{Error: "missing network"}
		}
		err = h.Connect(*req.Network)
	case "disconnect":
		err = h.Disconnect(req.Alias)
	case "forget":
		err = h.Forget(req.Alias)
	case "status":
	default:
		return Response{Error: "unknown op: " + req.Op}
	}
	// The full list goes back either way: a failed connect still changed what
	// the device knows, and the UI should see that.
	resp := Response{OK: err == nil, Networks: h.Statuses()}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp
}

// Call sends one request to the service and returns its response.
func Call(req Request) (Response, error) {
	timeout := 30 * time.Second
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

// Connect asks the service to join a network and remember it.
func Connect(n profile.Network) (Response, error) { return Call(Request{Op: "connect", Network: &n}) }

// Disconnect asks the service to take one network down, keeping it in the list.
func Disconnect(alias string) (Response, error) {
	return Call(Request{Op: "disconnect", Alias: alias})
}

// Forget asks the service to remove a network from the device entirely.
func Forget(alias string) (Response, error) { return Call(Request{Op: "forget", Alias: alias}) }

// Status asks the service for every known network.
func Status() (Response, error) { return Call(Request{Op: "status"}) }
