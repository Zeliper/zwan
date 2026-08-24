//go:build windows

// Command zwan-service is the SYSTEM engine service. It hosts the overlay engine
// and serves the user-mode tray/GUI over a named pipe. Run without arguments it
// behaves as a Windows service (when launched by the SCM); with a subcommand it
// installs/controls the service (requires Administrator).
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Zeliper/zwan/client/engine"
	"github.com/Zeliper/zwan/client/ipc"
	"github.com/Zeliper/zwan/client/profile"
	"github.com/Zeliper/zwan/client/tun"
	"github.com/Zeliper/zwan/shared"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName    = "zwanEngine"
	serviceDisplay = "zwan overlay engine"
	serviceDesc    = "Runs the zwan private overlay network (virtual adapter, encrypted tunnel, DNS)."
)

// handler implements ipc.Handler on top of an engine.
type handler struct{ eng *engine.Engine }

func (h *handler) Connect(a ipc.ConnectArgs) error {
	dev, err := profile.MachineDeviceUUID()
	if err != nil {
		return err
	}
	h.eng.Stop()
	return h.eng.Start(engine.Config{
		Server:      a.Server,
		Token:       a.Token,
		DeviceUUID:  dev,
		Name:        a.Name,
		UseRelay:    a.UseRelay,
		DNSAddr:     "127.0.0.1:53",
		ProductName: shared.ProductName,
	})
}

func (h *handler) Disconnect() error { h.eng.Stop(); return nil }
func (h *handler) Status() engine.Status { return h.eng.Status() }

// program is the svc.Handler.
type program struct{ h *handler }

func (p *program) Execute(_ []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	go func() {
		if err := ipc.Serve(p.h); err != nil {
			log.Printf("ipc serve: %v", err)
		}
	}()

	changes <- svc.Status{State: svc.Running, Accepts: accepted}
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			p.h.eng.Stop()
			return false, 0
		}
	}
	return false, 0
}

func main() {
	h := &handler{eng: engine.New()}

	isSvc, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("determine service context: %v", err)
	}
	if isSvc {
		_ = svc.Run(serviceName, &program{h})
		return
	}

	if len(os.Args) < 2 {
		fmt.Println("usage: zwan-service install|uninstall|start|stop|run")
		return
	}
	switch os.Args[1] {
	case "install":
		install()
	case "uninstall":
		uninstall()
	case "start":
		control("start")
	case "stop":
		control("stop")
	case "run": // foreground, for debugging
		log.Printf("%s (%s) running in console; Ctrl+C to stop", serviceDisplay, shared.Version)
		if err := ipc.Serve(h); err != nil {
			log.Fatal(err)
		}
	case "driver-install": // installs the Wintun driver by briefly creating an adapter
		ad, err := tun.Create("zwan-driversetup", tun.DefaultMTU)
		if err != nil {
			log.Fatalf("driver install (run as Administrator; wintun.dll present?): %v", err)
		}
		_ = ad.Close()
		log.Println("wintun driver installed")
	default:
		fmt.Println("unknown command:", os.Args[1])
	}
}

func install() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("executable path: %v", err)
	}
	m, err := mgr.Connect()
	if err != nil {
		log.Fatalf("connect to service manager (run as Administrator): %v", err)
	}
	defer m.Disconnect()

	if s, err := m.OpenService(serviceName); err == nil {
		s.Close()
		log.Fatalf("service %s already installed", serviceName)
	}
	s, err := m.CreateService(serviceName, exe, mgr.Config{
		DisplayName: serviceDisplay,
		Description: serviceDesc,
		StartType:   mgr.StartAutomatic,
	})
	if err != nil {
		log.Fatalf("create service: %v", err)
	}
	defer s.Close()
	if err := s.Start(); err != nil {
		log.Printf("service installed but failed to start: %v", err)
	}
	log.Printf("service %s installed and started", serviceName)
}

func uninstall() {
	m, err := mgr.Connect()
	if err != nil {
		log.Fatalf("connect to service manager (run as Administrator): %v", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		log.Fatalf("service %s not installed", serviceName)
	}
	defer s.Close()
	_, _ = s.Control(svc.Stop)
	if err := s.Delete(); err != nil {
		log.Fatalf("delete service: %v", err)
	}
	log.Printf("service %s removed", serviceName)
}

func control(action string) {
	m, err := mgr.Connect()
	if err != nil {
		log.Fatalf("connect to service manager (run as Administrator): %v", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		log.Fatalf("open service: %v", err)
	}
	defer s.Close()
	if action == "start" {
		if err := s.Start(); err != nil {
			log.Fatalf("start: %v", err)
		}
		log.Println("started")
		return
	}
	if _, err := s.Control(svc.Stop); err != nil {
		log.Fatalf("stop: %v", err)
	}
	log.Println("stopped")
}
