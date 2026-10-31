package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/rodrigocfd/windigo/co"
)

// checkForUpdate asks GitHub for the latest release in the background and
// offers to install it when it is newer than this build.
func (me *app) checkForUpdate() {
	if version == "" || !me.prefs.CheckUpdates {
		return
	}
	go func() {
		data, err := httpGet(latestReleaseURL, 1<<20)
		if err != nil {
			return // offline or rate limited: try again next start
		}
		r, ok := parseRelease(data)
		if !ok || !newerVersion(r.Version, version) {
			return
		}
		me.wnd.UiThread(func() { me.offerUpdate(r) })
	}()
}

func (me *app) offerUpdate(r release) {
	msg := fmt.Sprintf("otp4win %s is available (you have %s).\n\n"+
		"Download and install it now? otp4win closes during the installation.\n\n"+
		"Update checks can be turned off in Settings.", r.Version, version)
	if ret, _ := me.wnd.Hwnd().MessageBox(msg, "Update available", co.MB_YESNO|co.MB_ICONINFORMATION); ret != co.ID_YES {
		return
	}
	me.wnd.Hwnd().KillTimer(timerToast)
	me.lblStatus.Hwnd().SetWindowText("Downloading otp4win " + r.Version + "...")
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
				me.lblStatus.Hwnd().SetWindowText("")
				me.showError("Could not install the update", err)
				return
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
