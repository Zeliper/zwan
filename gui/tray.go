package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/Zeliper/zwan/client/manager"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// traySlots is how many networks the tray menu can show.
//
// systray can hide and rename items but not add them after startup, so the menu
// is built once and slots are bound to networks as they appear. Anything beyond
// this is still reachable from the window.
const traySlots = 8

// startTray runs the system tray icon and menu. It blocks (systray owns a
// message loop), so call it on its own goroutine.
//
// The tray covers both roles: the networks this device has joined, and the
// network this machine hosts. Either can be running with the window closed,
// which is the whole point of keeping the engines in services.
func startTray(app *App, icon []byte) {
	onReady := func() {
		if len(icon) > 0 {
			systray.SetIcon(icon)
		}
		systray.SetTitle("zwan")
		systray.SetTooltip("zwan overlay network")

		mShow := systray.AddMenuItem("Show", "Show the zwan window")
		systray.AddSeparator()

		slots := &networkSlots{app: app}
		for i := 0; i < traySlots; i++ {
			item := systray.AddMenuItem("", "")
			item.Hide()
			slots.items = append(slots.items, item)
			go slots.watch(i, item)
		}

		systray.AddSeparator()
		mStopHost := systray.AddMenuItem("Stop hosting", "Stop the network this machine hosts")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Quit zwan")

		go trackState(app, slots, mStopHost)

		for {
			select {
			case <-mShow.ClickedCh:
				if app.ctx != nil {
					wruntime.WindowShow(app.ctx)
				}
			case <-mStopHost.ClickedCh:
				_ = app.HostStop()
			case <-mQuit.ClickedCh:
				if app.ctx != nil {
					wruntime.Quit(app.ctx)
				} else {
					systray.Quit()
				}
				return
			}
		}
	}
	systray.Run(onReady, func() {})
}

// networkSlots binds tray menu entries to joined networks. Clicking one toggles
// it, so a network can be left or rejoined without opening the window.
type networkSlots struct {
	app   *App
	items []*systray.MenuItem

	mu    sync.Mutex
	bound []manager.Status // parallel to items; short entries mean unused slots
}

func (s *networkSlots) watch(i int, item *systray.MenuItem) {
	for range item.ClickedCh {
		s.mu.Lock()
		var st manager.Status
		if i < len(s.bound) {
			st = s.bound[i]
		}
		s.mu.Unlock()
		if st.Network.Alias == "" {
			continue
		}
		if st.Engine.Connected {
			_, _ = s.app.Disconnect(st.Network.Alias)
		} else {
			_, _ = s.app.Connect(st.Network)
		}
	}
}

// set rebinds the menu to the current network list.
func (s *networkSlots) set(list []manager.Status) {
	s.mu.Lock()
	s.bound = list
	s.mu.Unlock()

	for i, item := range s.items {
		if i >= len(list) {
			item.Hide()
			continue
		}
		st := list[i]
		state := "disconnected"
		if st.Engine.Connected {
			state = st.Engine.AssignedIP
			if state == "" {
				state = "connected"
			}
		}
		item.SetTitle(fmt.Sprintf("%s — %s", st.Network.Alias, state))
		item.SetTooltip(clickHint(st))
		if st.Engine.Connected {
			item.Check()
		} else {
			item.Uncheck()
		}
		item.Show()
	}
}

func clickHint(st manager.Status) string {
	if st.Engine.Connected {
		return "Leave " + st.Network.Alias
	}
	return "Join " + st.Network.Alias
}

// trackState keeps the tooltip and the menu in step with what is actually
// running, so the tray is readable without opening the window.
func trackState(app *App, slots *networkSlots, stopHost *systray.MenuItem) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		nets, err := app.Networks()
		if err != nil {
			nets = nil
		}
		slots.set(nets)

		var joined []string
		for _, n := range nets {
			if n.Engine.Connected {
				joined = append(joined, n.Network.Alias)
			}
		}

		hosting := ""
		if st := app.HostStatus(); st != nil && st.Running {
			hosting = "hosting " + st.Config.NetworkID
			stopHost.Enable()
		} else {
			stopHost.Disable()
		}
		systray.SetTooltip(tooltip(joined, hosting))
		<-tick.C
	}
}

func tooltip(joined []string, hosting string) string {
	var parts []string
	if len(joined) > 0 {
		parts = append(parts, "joined "+strings.Join(joined, ", "))
	}
	if hosting != "" {
		parts = append(parts, hosting)
	}
	if len(parts) == 0 {
		return "zwan — idle"
	}
	return "zwan — " + strings.Join(parts, "; ")
}
