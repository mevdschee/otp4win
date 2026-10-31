package main

import "testing"

func TestNewerVersion(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"0.3.0", "0.2.0", true},
		{"v0.2.1", "0.2.0", true},
		{"0.10.0", "0.9.9", true},
		{"1.0", "0.9.9", true},
		{"0.2.0", "0.2.0", false},
		{"0.1.9", "0.2.0", false},
		{"0.2", "0.2.0", false},
		{"0.3.0", "", false},
		{"0.3.0-beta", "0.2.0", false},
	} {
		if got := newerVersion(tc.a, tc.b); got != tc.want {
			t.Errorf("newerVersion(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestParseRelease(t *testing.T) {
	data := []byte(`{
		"tag_name": "v0.3.0", "draft": false, "prerelease": false,
		"assets": [
			{"name": "notes.txt", "browser_download_url": "https://example.com/notes.txt", "size": 10},
			{"name": "otp4win-0.3.0-amd64.msi", "browser_download_url": "https://github.com/mevdschee/otp4win/releases/download/v0.3.0/otp4win-0.3.0-amd64.msi", "size": 3912192}
		]}`)
	r, ok := parseRelease(data)
	if !ok {
		t.Fatal("no release found")
	}
	want := release{Version: "0.3.0", MsiName: "otp4win-0.3.0-amd64.msi", MsiSize: 3912192,
		MsiURL: "https://github.com/mevdschee/otp4win/releases/download/v0.3.0/otp4win-0.3.0-amd64.msi"}
	if r != want {
		t.Errorf("got %+v", r)
	}

	for name, bad := range map[string]string{
		"no msi":     `{"tag_name": "v0.3.0", "assets": [{"name": "a.zip", "browser_download_url": "https://x/a.zip"}]}`,
		"prerelease": `{"tag_name": "v0.3.0", "prerelease": true, "assets": [{"name": "a.msi", "browser_download_url": "https://x/a.msi"}]}`,
		"not https":  `{"tag_name": "v0.3.0", "assets": [{"name": "a.msi", "browser_download_url": "http://x/a.msi"}]}`,
		"invalid":    `not json`,
	} {
		if _, ok := parseRelease([]byte(bad)); ok {
			t.Errorf("%s: expected no release", name)
		}
	}
}
