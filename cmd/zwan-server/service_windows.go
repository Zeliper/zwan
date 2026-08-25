//go:build windows

package main

import (
	"fmt"
	"log"
	"os"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/Zeliper/zwan/server/ipc"
	"github.com/Zeliper/zwan/server/supervisor"
	"github.com/Zeliper/zwan/shared"
	"github.com/Zeliper/zwan/shared/firewall"
)

const (
	serviceName    = "zwanServer"
	serviceDisplay = "zwan control server"
	serviceDesc    = "Hosts a zwan private overlay network (control plane, DNS/service registry and relay)."

	// The firewall rule is found again by this name when the service is
	// removed, so it is fixed rather than derived from anything configurable.
	firewallRuleName = "zwan control server"
)

// program is the svc.Handler wrapping the supervisor.
type program struct{ sup *supervisor.Supervisor }

func (p *program) Execute(_ []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	go func() {
		if err := ipc.Serve(p.sup); err != nil {
			log.Printf("ipc serve: %v", err)
		}
	}()
	go p.sup.AutoStart()

	changes <- svc.Status{State: svc.Running, Accepts: accepted}
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			p.sup.Shutdown()
			return false, 0
		}
	}
	return false, 0
}

// runAsService runs the control server under the SCM when that is how we were
// launched, and reports whether it did.
func runAsService() bool {
	isSvc, err := svc.IsWindowsService()
	if err != nil || !isSvc {
		return false
	}
	_ = svc.Run(serviceName, &program{sup: supervisor.New()})
	return true
}

// handleServiceCommand implements "zwan-server service <cmd>", returning false
// when the arguments are not a service command.
func handleServiceCommand(args []string) bool {
	if len(args) == 0 || args[0] != "service" {
		return false
	}
	if len(args) < 2 {
		fmt.Println("usage: zwan-server service install|uninstall|start|stop|run")
		return true
	}
	switch args[1] {
	case "install":
		installService()
	case "uninstall":
		uninstallService()
	case "start":
		controlService("start")
	case "stop":
		controlService("stop")
	case "run": // foreground, for debugging
		log.Printf("%s (%s) running in console; Ctrl+C to stop", serviceDisplay, shared.Version)
		sup := supervisor.New()
		go sup.AutoStart()
		if err := ipc.Serve(sup); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Println("unknown service command:", args[1])
	}
	return true
}

func installService() {
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
	// A service running as SYSTEM never gets the "allow this app?" prompt, so
	// without this its inbound packets are dropped while it sits there bound and
	// apparently healthy. The only symptom is a client elsewhere timing out.
	if err := firewall.Allow(firewallRule(exe)); err != nil {
		log.Printf("firewall: %v", err)
		log.Printf("clients will not reach this server until inbound is allowed for %s", exe)
	} else {
		log.Printf("firewall: inbound allowed for %s", exe)
	}
	log.Printf("service %s installed and started", serviceName)
}

// firewallRule is the allowance this service needs. It names the program rather
// than a port because the control and relay ports are both configuration, and a
// rule for the wrong port is as silent as no rule at all.
func firewallRule(exe string) firewall.Rule {
	return firewall.Rule{
		Name:        firewallRuleName,
		Exe:         exe,
		Description: "Inbound for the " + shared.ProductName + " control server and relay.",
	}
}

func uninstallService() {
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
	// The rule is machine state and outlives the service, so removing the
	// service has to take it out too.
	if err := firewall.Remove(firewallRuleName); err != nil {
		log.Printf("firewall: %v", err)
	}
	log.Printf("service %s removed", serviceName)
}

func controlService(action string) {
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
