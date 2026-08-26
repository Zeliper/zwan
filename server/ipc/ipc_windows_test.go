//go:build windows

package ipc

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zeliper/zwan/server/config"
)

// fakeHandler stands in for the service so the pipe itself is what gets tested.
type fakeHandler struct {
	mu       sync.Mutex
	started  *config.Config
	stops    int
	failWith error
}

func (f *fakeHandler) Start(c config.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return f.failWith
	}
	f.started = &c
	return nil
}

func (f *fakeHandler) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	return nil
}

func (f *fakeHandler) State() State {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := State{JoinURL: "https://example.test:8787"}
	if f.started != nil {
		st.Running = true
		st.Config = *f.started
	}
	return st
}

// TestPipeRoundTrip covers the whole channel in one test: the pipe name is a
// fixed global, so a second concurrent listener would collide.
func TestPipeRoundTrip(t *testing.T) {
	requireFreePipe(t)

	h := &fakeHandler{}
	serveErr := make(chan error, 1)
	go func() { serveErr <- Serve(h) }()

	waitForPipe(t, serveErr)

	if resp, err := Status(); err != nil || !resp.OK {
		t.Fatalf("status: err=%v resp=%+v", err, resp)
	} else if resp.State.Running {
		t.Fatal("nothing has been started yet")
	}

	cfg := config.Default()
	cfg.Token = "join-token"
	cfg.NetworkID = "home"
	resp, err := Start(cfg)
	if err != nil || !resp.OK {
		t.Fatalf("start: err=%v resp=%+v", err, resp)
	}
	if !resp.State.Running || resp.State.Config.Token != "join-token" {
		t.Fatalf("start state = %+v", resp.State)
	}

	if resp, err := Status(); err != nil || !resp.State.Running {
		t.Fatalf("status after start: err=%v resp=%+v", err, resp)
	} else if resp.State.JoinURL == "" {
		t.Fatal("status should carry the join URL")
	}

	if resp, err := Stop(); err != nil || !resp.OK {
		t.Fatalf("stop: err=%v resp=%+v", err, resp)
	}
	h.mu.Lock()
	stops := h.stops
	h.mu.Unlock()
	if stops != 1 {
		t.Fatalf("stop calls = %d, want 1", stops)
	}

	// A handler error travels back as a message, not a transport failure.
	h.mu.Lock()
	h.failWith = errFake
	h.mu.Unlock()
	resp, err = Start(cfg)
	if err != nil {
		t.Fatalf("a rejected start should still round-trip: %v", err)
	}
	if resp.OK || !strings.Contains(resp.Error, "no token") {
		t.Fatalf("rejected start = %+v", resp)
	}

	if resp, err := Call(Request{Op: "nonsense"}); err != nil || resp.OK {
		t.Fatalf("unknown op should be reported: err=%v resp=%+v", err, resp)
	}
}

var errFake = fakeErr("no token configured")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

// requireFreePipe refuses to run when something already answers on the pipe.
//
// The name is a fixed global, so an installed zwanServer service owns it on any
// machine the product is installed on — which is the machine this is most likely
// to be run on. Serve then fails to listen, and the test carries on regardless:
// the early assertions pass because the real service answers them, and the rest
// of the test drives it. Start really starts a network and Stop really stops
// one, on the developer's own machine, while the failure that eventually
// surfaces is an unrelated-looking count of calls the fake never received.
//
// waitForPipe cannot catch this on its own. It checks the error channel before
// each dial, but the first dial succeeds — answered by the service — and returns
// before Serve has got round to reporting that the name was taken.
func requireFreePipe(t *testing.T) {
	t.Helper()
	if _, err := Status(); err == nil {
		t.Skipf("something already answers on %s (the zwanServer service?); "+
			"stop it before running this test, or it would be the thing under test", PipeName)
	}
}

func waitForPipe(t *testing.T, serveErr <-chan error) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		select {
		case err := <-serveErr:
			t.Fatalf("listen on %s: %v", PipeName, err)
		default:
		}
		if _, err := Status(); err == nil {
			return
		} else {
			last = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the IPC pipe never came up (last dial error: %v)", last)
}
