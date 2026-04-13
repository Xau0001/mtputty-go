package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const githubOwner = "Xau0001"
const githubRepo = "mtssh"

type githubRelease struct {
	TagName string        `json:"tag_name"`
	HTMLURL string        `json:"html_url"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// LatestRelease fetches the newest GitHub release.
// Returns (version without "v" prefix, binary download URL for the current OS,
// release page URL, error). downloadURL is empty if no matching asset was found.
func LatestRelease() (version, downloadURL, pageURL string, err error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", githubOwner, githubRepo)
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "mtssh-updater")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = fmt.Errorf("GitHub API: HTTP %d", resp.StatusCode)
		return
	}

	var rel githubRelease
	if err = json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return
	}

	version = strings.TrimPrefix(rel.TagName, "v")
	pageURL = rel.HTMLURL

	// Expected asset names: "mtssh-linux-amd64", "mtssh-windows-amd64.exe"
	token := platformToken()
	for _, a := range rel.Assets {
		if strings.Contains(a.Name, token) {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	return
}

func platformToken() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

// IsNewer reports whether latest is strictly newer than current (semver).
func IsNewer(current, latest string) bool {
	c := parseSemver(current)
	l := parseSemver(latest)
	for i := range l {
		if i >= len(c) {
			return true
		}
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

func parseSemver(v string) []int {
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	out := make([]int, len(parts))
	for i, p := range parts {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

// SelfUpdate downloads the binary at url and atomically replaces the running
// executable. progress is called with values in [0, 1] during the download.
// On Windows os.Rename of a running binary fails — callers should open the
// release page instead (LatestRelease returns pageURL for this purpose).
func SelfUpdate(url string, progress func(float64)) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	tmp := exe + ".update"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}

	total := resp.ContentLength
	var done int64
	buf := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				os.Remove(tmp)
				return fmt.Errorf("write: %w", werr)
			}
			done += int64(n)
			if progress != nil && total > 0 {
				progress(float64(done) / float64(total))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("read: %w", rerr)
		}
	}
	f.Close()

	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("failed to replace binary (missing permissions?): %w", err)
	}
	return nil
}
