//go:build windows

// Command zwan-service is the SYSTEM engine service. It runs the overlay engines
// and serves the user-mode tray/GUI over a named pipe. Run without arguments it
// behaves as a Windows service (when launched by the SCM); with a subcommand it
// installs/controls the service (requires Administrator).
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/Zeliper/zwan/client/ipc"
	"github.com/Zeliper/zwan/client/manager"
	"github.com/Zeliper/zwan/client/nrpt"
	"github.com/Zeliper/zwan/client/profile"
	"github.com/Zeliper/zwan/client/tun"
	"github.com/Zeliper/zwan/shared"
	"github.com/Zeliper/zwan/shared/firewall"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName    = "zwanEngine"
	serviceDisplay = "zwan overlay engine"
	serviceDesc    = "Runs the zwan private overlay network (virtual adapter, encrypted tunnel, DNS)."

	// Fixed, because removing the service has to find this rule again.
	firewallRuleName = "zwan overlay engine"
)

// newManager builds the supervisor for every network this device has joined.
//
// The device identifier is machine-wide: the SYSTEM service registers on behalf
// of the machine, so the same overlay addresses come back after a restart.
func newManager() (*manager.Manager, error) {
	dev, err := profile.MachineDeviceUUID()
	if err != nil {
		return nil, fmt.Errorf("device identity: %w", err)
	}
	return manager.New(manager.Config{
		DNSAddr:     "127.0.0.1:53",
		ProductName: shared.ProductName,
		DeviceUUID:  dev,
	}), nil
}

// program is the svc.Handler.
type program struct{ mgr *manager.Manager }

func (p *program) Execute(_ []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	go func() {
		if err := ipc.Serve(p.mgr); err != nil {
			log.Printf("ipc serve: %v", err)
		}
	}()
	go p.mgr.Start()

	changes <- svc.Status{State: svc.Running, Accepts: accepted}
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			p.mgr.Stop()
			return false, 0
		}
	}
	return false, 0
}

func main() {
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("determine service context: %v", err)
	}
	if isSvc {
		mgr, err := newManager()
		if err != nil {
			log.Fatal(err)
		}
		_ = svc.Run(serviceName, &program{mgr: mgr})
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
		mgr, err := newManager()
		if err != nil {
			log.Fatal(err)
		}
		// Ctrl+C has to run the same teardown the service does: the engine
		// leaves machine state behind — adapters, routes, name resolution
		// policy — that no one else will clean up.
		go func() {
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt)
			<-c
			log.Print("stopping")
			mgr.Stop()
			os.Exit(0)
		}()
		go mgr.Start()
		if err := ipc.Serve(mgr); err != nil {
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
	// SYSTEM services get no firewall prompt, so a direct tunnel would be
	// dropped on the way in with nothing to say why.
	if err := firewall.Allow(firewallRule(exe)); err != nil {
		log.Printf("firewall: %v", err)
		log.Printf("peers may not reach this device directly until inbound is allowed for %s", exe)
	} else {
		log.Printf("firewall: inbound allowed for %s", exe)
	}
	log.Printf("service %s installed and started", serviceName)
}

// firewallRule is the allowance this service needs. The tunnel's UDP port is
// chosen per network and changes as networks are joined, so the rule names the
// program instead.
func firewallRule(exe string) firewall.Rule {
	return firewall.Rule{
		Name:        firewallRuleName,
		Exe:         exe,
		Description: "Inbound for the " + shared.ProductName + " overlay engine.",
	}
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
	if err := firewall.Remove(firewallRuleName); err != nil {
		log.Printf("firewall: %v", err)
	}
	// Name resolution policy rules are machine state, not the service's own:
	// removing the service has to leave the machine resolving names as it did
	// before, including after a run that was killed rather than stopped.
	if err := nrpt.Purge(); err != nil {
		log.Printf("clear name resolution policy: %v", err)
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
