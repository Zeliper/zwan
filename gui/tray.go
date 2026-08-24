package main

import (
	"fmt"
	"time"

	"fyne.io/systray"

	clientipc "github.com/Zeliper/zwan/client/ipc"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// startTray runs the system tray icon and menu. It blocks (systray owns a
// message loop), so call it on its own goroutine.
//
// The tray covers both roles: the network this device joined, and the network
// this machine hosts. Either can be running with the window closed, which is
// the whole point of keeping the engines in services.
func startTray(app *App, icon []byte) {
	onReady := func() {
		if len(icon) > 0 {
			systray.SetIcon(icon)
		}
		systray.SetTitle("zwan")
		systray.SetTooltip("zwan overlay network")

		mShow := systray.AddMenuItem("Show", "Show the zwan window")
		systray.AddSeparator()
		mDisconnect := systray.AddMenuItem("Disconnect", "Leave the network this device joined")
		mStopHost := systray.AddMenuItem("Stop hosting", "Stop the network this machine hosts")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Quit zwan")

		go trackState(app, mDisconnect, mStopHost)

		for {
			select {
			case <-mShow.ClickedCh:
				if app.ctx != nil {
					wruntime.WindowShow(app.ctx)
				}
			case <-mDisconnect.ClickedCh:
				_, _ = clientipc.Disconnect()
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

// trackState keeps the tooltip and the menu in step with what is actually
// running, so the tray is readable without opening the window.
func trackState(app *App, disconnect, stopHost *systray.MenuItem) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		joined, hosting := "", ""
		if resp, err := clientipc.Status(); err == nil && resp.Status != nil && resp.Status.Connected {
			joined = fmt.Sprintf("joined %s as %s", resp.Status.NetworkID, resp.Status.AssignedIP)
			disconnect.Enable()
		} else {
			disconnect.Disable()
		}
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

func tooltip(joined, hosting string) string {
	switch {
	case joined != "" && hosting != "":
		return "zwan — " + joined + "; " + hosting
	case joined != "":
		return "zwan — " + joined
	case hosting != "":
		return "zwan — " + hosting
	default:
		return "zwan — idle"
	}
}
