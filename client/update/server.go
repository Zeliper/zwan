package update

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/minio/selfupdate"
)

// ServerAssetName is the release asset name for the running OS/arch server binary.
func ServerAssetName() string {
	n := fmt.Sprintf("zwan-server-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		n += ".exe"
	}
	return n
}

func latestServerAsset() (version, url string, err error) {
	gr, err := fetchLatest()
	if err != nil {
		return "", "", err
	}
	version = strings.TrimPrefix(gr.TagName, "v")
	name := ServerAssetName()
	for _, a := range gr.Assets {
		if a.Name == name {
			return version, a.URL, nil
		}
	}
	return version, "", fmt.Errorf("no server asset %q in the latest release", name)
}

// SelfUpdateServer replaces the running server binary with the latest release,
// if it is newer than current. It returns the latest version and whether an
// update was applied. After a successful update the caller must restart to run
// the new binary.
func SelfUpdateServer(current string) (latest string, updated bool, err error) {
	ver, url, err := latestServerAsset()
	if err != nil {
		return "", false, err
	}
	if !IsNewer(current, ver) {
		return ver, false, nil
	}
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Get(url)
	if err != nil {
		return ver, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ver, false, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	if err := selfupdate.Apply(resp.Body, selfupdate.Options{}); err != nil {
		return ver, false, err
	}
	return ver, true, nil
}
