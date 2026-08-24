// Package update checks GitHub releases for a newer version and applies it by
// downloading the latest installer and running it silently (an automatic update,
// not a manual reinstall). See apply_windows.go for the platform apply step.
package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Repo coordinates. Overridable at build time via -ldflags if the fork differs.
var (
	Owner = "Zeliper"
	Repo  = "zwan"
)

// Release is the newest release relevant to updating.
type Release struct {
	Tag          string `json:"tag"`
	Version      string `json:"version"`
	InstallerURL string `json:"installerUrl"`
	Notes        string `json:"notes"`
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Latest returns the newest published release and its installer asset URL.
func Latest() (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", Owner, Repo)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github releases: %s", resp.Status)
	}
	var gr ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, err
	}
	rel := &Release{Tag: gr.TagName, Version: strings.TrimPrefix(gr.TagName, "v"), Notes: gr.Body}
	for _, a := range gr.Assets {
		n := strings.ToLower(a.Name)
		if strings.HasSuffix(n, ".exe") && (strings.Contains(n, "setup") || strings.Contains(n, "installer")) {
			rel.InstallerURL = a.URL
			break
		}
	}
	return rel, nil
}

// IsNewer reports whether latest is a newer version than current. A dev/empty
// current is always considered older (so testers always see updates).
func IsNewer(current, latest string) bool {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")
	if latest == "" {
		return false
	}
	if current == "" || strings.Contains(current, "dev") {
		return true
	}
	c, l := parse(current), parse(latest)
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parse(v string) [3]int {
	base := strings.SplitN(v, "-", 2)[0]
	base = strings.SplitN(base, "+", 2)[0]
	parts := strings.Split(base, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		out[i], _ = strconv.Atoi(strings.TrimSpace(parts[i]))
	}
	return out
}
