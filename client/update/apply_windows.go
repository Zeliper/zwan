//go:build windows

package update

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// Apply downloads the installer and launches it silently and elevated (/S). A
// UAC prompt appears; on approval the installer updates the files in place and
// restarts the service. The caller should exit shortly after so files unlock.
func Apply(installerURL string) error {
	if installerURL == "" {
		return fmt.Errorf("release has no installer asset")
	}
	tmp := filepath.Join(os.TempDir(), "zwan-setup.exe")
	if err := download(installerURL, tmp); err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(tmp)
	args, _ := windows.UTF16PtrFromString("/S")
	return windows.ShellExecute(0, verb, file, args, nil, windows.SW_SHOWNORMAL)
}

func download(url, dst string) error {
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
