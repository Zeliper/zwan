package main

import (
	"fyne.io/systray"

	"github.com/Zeliper/zwan/client/ipc"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// startTray runs the system tray icon and menu. It blocks (systray owns a
// message loop), so call it on its own goroutine.
func startTray(app *App, icon []byte) {
	onReady := func() {
		if len(icon) > 0 {
			systray.SetIcon(icon)
		}
		systray.SetTitle("zwan")
		systray.SetTooltip("zwan overlay network")

		mShow := systray.AddMenuItem("Show", "Show the zwan window")
		mDisconnect := systray.AddMenuItem("Disconnect", "Disconnect the overlay")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Quit zwan")

		for {
			select {
			case <-mShow.ClickedCh:
				if app.ctx != nil {
					wruntime.WindowShow(app.ctx)
				}
			case <-mDisconnect.ClickedCh:
				_, _ = ipc.Disconnect()
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
