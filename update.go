package main

import (
	"encoding/json"
	"strconv"
	"strings"
)

// version is set at build time by release.sh (-ldflags -X main.version=...).
// Development builds have no version and skip the update check.
var version = ""

// latestReleaseURL is the GitHub API endpoint for the newest release.
const latestReleaseURL = "https://api.github.com/repos/mevdschee/otp4win/releases/latest"

// release is the part of the GitHub release JSON the update check needs.
type release struct {
	Version string // without the leading "v"
	MsiURL  string
	MsiName string
	MsiSize int64
}

// parseRelease reads a GitHub "latest release" response and finds the MSI.
func parseRelease(data []byte) (release, bool) {
	var r struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(data, &r); err != nil || r.Draft || r.Prerelease {
		return release{}, false
	}
	for _, a := range r.Assets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".msi") && strings.HasPrefix(a.URL, "https://") {
			return release{
				Version: strings.TrimPrefix(r.TagName, "v"),
				MsiURL:  a.URL, MsiName: a.Name, MsiSize: a.Size,
			}, true
		}
	}
	return release{}, false
}

// newerVersion reports whether version a is newer than b. Versions are
// dotted numbers like "0.2.0"; anything else is never newer.
func newerVersion(a, b string) bool {
	pa, okA := parseVersion(a)
	pb, okB := parseVersion(b)
	if !okA || !okB {
		return false
	}
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func parseVersion(s string) ([]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if s == "" {
		return nil, false
	}
	var parts []int
	for _, p := range strings.Split(s, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		parts = append(parts, n)
	}
	return parts, true
}
