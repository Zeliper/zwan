// Package supervisor owns a hosted network and the saved configuration behind
// it. The Windows service runs one, and the desktop app runs one in-process
// when the service is not installed, so both paths behave identically.
package supervisor

import (
	"log"
	"sync"

	"github.com/Zeliper/zwan/server/config"
	"github.com/Zeliper/zwan/server/host"
	"github.com/Zeliper/zwan/server/ipc"
)

// Supervisor is a start/stoppable hosted network. It satisfies ipc.Handler.
//
// Every entry point takes the mutex: host.Host is not safe to reconfigure and
// read at the same time, and the IPC server dispatches each request on its own
// goroutine.
type Supervisor struct {
	mu      sync.Mutex
	srv     *host.Host
	cfg     config.Config
	lastErr string
}

// New loads the saved configuration and returns an idle supervisor.
func New() *Supervisor {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("server: load config: %v (starting from defaults)", err)
	}
	return &Supervisor{srv: host.New(), cfg: cfg}
}

// AutoStart brings the saved network up if the operator left it running.
func (s *Supervisor) AutoStart() {
	s.mu.Lock()
	cfg, want := s.cfg, s.cfg.AutoStart
	s.mu.Unlock()
	if !want {
		return
	}
	if err := s.Start(cfg); err != nil {
		log.Printf("server: auto-start: %v", err)
	}
}

// Config returns the saved configuration, so a UI can populate its form.
func (s *Supervisor) Config() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Start persists the configuration and brings the network up. Starting is what
// marks a network as wanted, so it comes back after a reboot.
func (s *Supervisor) Start(c config.Config) error {
	if err := c.Valid(); err != nil {
		s.mu.Lock()
		s.lastErr = err.Error()
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.srv.Stop()
	c.AutoStart = true
	if err := s.srv.Start(c.Host()); err != nil {
		s.cfg, s.lastErr = c, err.Error()
		return err
	}
	s.cfg, s.lastErr = c, ""
	s.save()
	log.Printf("server: hosting %s on %s (%s)", c.NetworkID, c.ControlAddr, s.srv.TLSMode())
	return nil
}

// Stop takes the network down and records that it should stay down.
func (s *Supervisor) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.srv.Stop()
	s.cfg.AutoStart = false
	s.lastErr = ""
	s.save()
	log.Print("server: stopped hosting")
	return nil
}

// Shutdown takes the network down without clearing auto-start: the machine is
// going away, the operator did not ask for the network to stay down.
func (s *Supervisor) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.srv.Stop()
}

// State reports the hosted network for the UI.
func (s *Supervisor) State() ipc.State {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := ipc.State{Config: s.cfg, Running: s.srv.Running(), LastError: s.lastErr}
	if st.Running {
		st.TLSMode = s.srv.TLSMode()
		st.Pin = s.srv.Pin()
		st.JoinURL = s.srv.JoinURL(s.cfg.PublicHost)
		st.Peers = s.srv.Members()
		st.Services = s.srv.Services()
	}
	return st
}

// save persists the current configuration; callers hold the mutex.
func (s *Supervisor) save() {
	if err := config.Save(s.cfg); err != nil {
		log.Printf("server: save config: %v", err)
	}
}
