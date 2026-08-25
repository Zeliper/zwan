package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

func main() {
	app := NewApp()

	// The operating system's language is the best guess until the window opens
	// and reports what the user actually chose.
	initLanguage()

	// System tray runs alongside the window; closing the window hides to tray.
	go startTray(app, trayIcon)

	err := wails.Run(&options.App{
		Title:             "zwan",
		Width:             1024,
		Height:            720,
		MinWidth:          720,
		MinHeight:         560,
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 10, G: 12, B: 16, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
