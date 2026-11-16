package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/Sebas1705/WinForge/internal/sched"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is stamped at release time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	// The scheduled task starts this program as `WinForge.exe updates --scheduled ...` when
	// winforge-cli.exe is not installed next to it. That is a headless run: no window, same code.
	if sched.IsUpdatesCommand(os.Args) {
		os.Exit(sched.RunCLI(os.Args[2:], os.Stdout, os.Stderr, sched.Deps{}))
	}
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "WinForge",
		Width:     1180,
		Height:    780,
		MinWidth:  980,
		MinHeight: 620,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 26, B: 31, A: 1},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
