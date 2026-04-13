package main

import (
	_ "embed"
	"fmt"
	"mtssh/config"
	"mtssh/core"
	"mtssh/logger"
	"mtssh/ui"
	"net/url"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

//go:embed icon.png
var iconData []byte

// Version is set at build time via -ldflags "-X main.Version=x.y.z"
var Version = "dev"

func main() {
	_ = logger.Init()
	defer logger.Close()

	a := app.New()
	a.SetIcon(fyne.NewStaticResource("icon.png", iconData))

	unlockWin := a.NewWindow("MTSSH — Unlock")
	unlockWin.Resize(fyne.NewSize(480, 220))

	passEntry := widget.NewPasswordEntry()
	passEntry.SetPlaceHolder("Enter master passphrase")

	unlock := func() {
		pass := passEntry.Text
		if pass == "" {
			dialog.ShowError(fmt.Errorf("passphrase must not be empty"), unlockWin)
			return
		}
		config.Init(pass)
		sessions, err := config.Load()
		if err != nil {
			dialog.ShowError(err, unlockWin)
			return
		}
		logger.Info("app", "session store unlocked")
		unlockWin.Hide()

		mainWin := ui.MainWindow(a, sessions, func(updated []config.Session) error {
			return config.Save(updated)
		})
		mainWin.Show()
		go checkForUpdates(a, mainWin, Version)
	}

	passEntry.OnSubmitted = func(_ string) { unlock() }

	unlockWin.SetContent(container.NewVBox(
		widget.NewLabel("MTSSH — Multi-Tabbed SSH Client"),
		widget.NewLabel("Enter your master passphrase to unlock the session store."),
		widget.NewLabel("First launch: choose any passphrase — it encrypts your sessions."),
		passEntry,
		widget.NewButton("Unlock", unlock),
	))

	unlockWin.ShowAndRun()
}

func checkForUpdates(a fyne.App, win fyne.Window, currentVersion string) {
	if currentVersion == "dev" {
		return
	}
	latest, downloadURL, pageURL, err := core.LatestRelease()
	if err != nil {
		logger.Error("updater", "update check: "+err.Error())
		return
	}
	if !core.IsNewer(currentVersion, latest) {
		return
	}

	// Windows: os.Rename on a running binary fails — open the release page instead.
	if runtime.GOOS == "windows" || downloadURL == "" {
		msg := fmt.Sprintf("Version %s is available (current: %s).\nOpen in browser?", latest, currentVersion)
		dialog.ShowConfirm("Update Available", msg, func(ok bool) {
			if !ok {
				return
			}
			u, parseErr := url.Parse(pageURL)
			if parseErr == nil {
				a.OpenURL(u)
			}
		}, win)
		return
	}

	// Linux / macOS: self-update with progress bar.
	msg := fmt.Sprintf("Version %s is available (current: %s).\nUpdate now?", latest, currentVersion)
	dialog.ShowConfirm("Update Available", msg, func(ok bool) {
		if !ok {
			return
		}
		prog := widget.NewProgressBar()
		status := widget.NewLabel("Downloading...")
		dlg := dialog.NewCustom("Updating MTSSH", "Close",
			container.NewVBox(status, prog), win)
		dlg.Show()

		go func() {
			updateErr := core.SelfUpdate(downloadURL, func(p float64) {
				prog.SetValue(p)
			})
			if updateErr != nil {
				status.SetText("Error: " + updateErr.Error())
				logger.Error("updater", updateErr.Error())
				return
			}
			prog.SetValue(1)
			status.SetText("Done! Please restart MTSSH.")
			logger.Info("updater", "updated to "+latest)
		}()
	}, win)
}
