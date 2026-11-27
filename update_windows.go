package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"golang.org/x/sys/windows"
)

// checkForUpdates is the "Check for updates" link in Settings: it asks
// GitHub for the latest release and offers to install it when it is newer.
// Nothing is checked automatically. The link shows the progress; dlg owns
// the message boxes.
func (me *app) checkForUpdates(dlg *ui.Modal, lnk *ui.SysLink) {
	h := dlg.Hwnd()
	if version == "" {
		h.MessageBox("This is a development build. Updates are only for installed releases.", "otp4win", co.MB_ICONINFORMATION)
		return
	}
	busy(lnk, "Checking for updates...")
	go func() {
		data, err := httpGet(latestReleaseURL, 1<<20)
		var r release
		var ok bool
		if err == nil {
			r, ok = parseRelease(data)
		}
		me.wnd.UiThread(func() {
			if dlg.Hwnd() == 0 {
				return // Settings was closed meanwhile
			}
			done(lnk)
			switch {
			case err != nil:
				h.MessageBox("Could not check for updates:\n"+err.Error(), "otp4win", co.MB_ICONERROR)
			case !ok || !newerVersion(r.Version, version):
				h.MessageBox("otp4win is up to date. You have the latest version ("+version+").", "otp4win", co.MB_ICONINFORMATION)
			default:
				me.offerUpdate(dlg, lnk, r)
			}
		})
	}()
}

const linkText = "<a>Check for updates</a>"

func busy(lnk *ui.SysLink, text string) {
	lnk.Hwnd().SetWindowText(text)
	lnk.Hwnd().EnableWindow(false)
}

func done(lnk *ui.SysLink) {
	lnk.Hwnd().SetWindowText(linkText)
	lnk.Hwnd().EnableWindow(true)
}

// installed reports whether this is the copy the MSI installed, in
// %LOCALAPPDATA%\Programs\otp4win, and not the portable exe. A portable copy
// cannot update itself: the MSI would install a second copy next to it.
func installed() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "otp4win")
	return strings.EqualFold(filepath.Clean(filepath.Dir(exe)), filepath.Clean(dir))
}

func (me *app) offerUpdate(dlg *ui.Modal, lnk *ui.SysLink, r release) {
	h := dlg.Hwnd()
	if !installed() {
		msg := fmt.Sprintf("otp4win %s is available (you have %s).\n\n"+
			"This copy is not installed, so it cannot update itself. Open the download page?", r.Version, version)
		if ret, _ := h.MessageBox(msg, "Update available", co.MB_YESNO|co.MB_ICONINFORMATION); ret == co.ID_YES {
			_ = windows.ShellExecute(windows.Handle(h), windows.StringToUTF16Ptr("open"),
				windows.StringToUTF16Ptr("https://github.com/mevdschee/otp4win/releases/latest"), nil, nil, windows.SW_SHOWNORMAL)
		}
		return
	}
	msg := fmt.Sprintf("otp4win %s is available (you have %s).\n\n"+
		"Download and install it now? otp4win closes during the installation.", r.Version, version)
	if ret, _ := h.MessageBox(msg, "Update available", co.MB_YESNO|co.MB_ICONINFORMATION); ret != co.ID_YES {
		return
	}
	busy(lnk, "Downloading otp4win "+r.Version+"...")
	go func() {
		path, err := downloadMsi(r)
		me.wnd.UiThread(func() {
			if err == nil {
				// msiexec shows the installer; the MSI replaces the running
				// version, so close to release otp4win.exe.
				log := filepath.Join(os.TempDir(), "otp4win-update.log")
				err = exec.Command("msiexec", "/i", path, "/l*v", log).Start()
			}
			if err != nil {
				if dlg.Hwnd() != 0 {
					done(lnk)
				}
				me.showError("Could not install the update", err)
				return
			}
			if dlg.Hwnd() != 0 {
				dlg.Hwnd().SendMessage(co.WM_CLOSE, 0, 0)
			}
			me.wnd.Hwnd().PostMessage(co.WM_CLOSE, 0, 0)
		})
	}()
}

func httpGet(url string, limit int64) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "otp4win/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// downloadMsi saves the MSI in the temp folder and checks its size.
func downloadMsi(r release) (string, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(r.MsiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}
	path := filepath.Join(os.TempDir(), filepath.Base(r.MsiName))
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, 200<<20))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && r.MsiSize > 0 && n != r.MsiSize {
		err = fmt.Errorf("download incomplete: %d of %d bytes", n, r.MsiSize)
	}
	if err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}
