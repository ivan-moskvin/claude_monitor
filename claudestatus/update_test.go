package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ivan-moskvin/claudestatus/internal/testenv"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		value string
		want  semver
		valid bool
	}{
		{"v1.2.3", semver{1, 2, 3}, true},
		{"1.2.3", semver{1, 2, 3}, true},
		{"  v1.2.3\n", semver{1, 2, 3}, true},
		{"v0.0.0", semver{}, true},
		{"v10.20.30", semver{10, 20, 30}, true},
		// A pre-release suffix does not take part in the comparison.
		{"v1.2.3-rc1", semver{1, 2, 3}, true},
		{"v1.2.3+build7", semver{1, 2, 3}, true},
		// Anything that is not three numbers has no version to compare.
		{"", semver{}, false},
		{"v1.2", semver{}, false},
		{"v1.2.3.4", semver{}, false},
		{"v1.2.x", semver{}, false},
		{"v-1.2.3", semver{}, false},
		{"v1.-2.3", semver{}, false},
		{"built from source", semver{}, false},
		{"сборка из исходников", semver{}, false},
	}

	for _, c := range cases {
		got, valid := parseVersion(c.value)
		if got != c.want || valid != c.valid {
			t.Errorf("parseVersion(%q) = %+v, %t; want %+v, %t", c.value, got, valid, c.want, c.valid)
		}
	}
}

func TestSemverLess(t *testing.T) {
	cases := []struct {
		left, right semver
		less        bool
	}{
		{semver{1, 2, 3}, semver{1, 2, 3}, false},
		{semver{1, 2, 3}, semver{1, 2, 4}, true},
		{semver{1, 2, 4}, semver{1, 2, 3}, false},
		{semver{1, 2, 9}, semver{1, 3, 0}, true},
		{semver{1, 9, 9}, semver{2, 0, 0}, true},
		// A bigger patch does not make up for a smaller minor.
		{semver{1, 2, 99}, semver{1, 3, 0}, true},
		{semver{2, 0, 0}, semver{1, 99, 99}, false},
	}

	for _, c := range cases {
		if got := c.left.less(c.right); got != c.less {
			t.Errorf("%+v.less(%+v) = %t; want %t", c.left, c.right, got, c.less)
		}
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		current, tag string
		want         bool
	}{
		{"v1.2.3", "v1.2.4", true},
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.4", "v1.2.3", false},
		{"v1.2.3", "v2.0.0", true},
		// A build made outside a release has no number: it never lights the mark
		// up, however new the tag is.
		{devVersion, "v9.9.9", false},
		{"", "v9.9.9", false},
		// Nor does a tag we cannot read.
		{"v1.2.3", "latest", false},
		{"v1.2.3", "", false},
	}

	for _, c := range cases {
		if got := newer(c.current, c.tag); got != c.want {
			t.Errorf("newer(%q, %q) = %t; want %t", c.current, c.tag, got, c.want)
		}
	}
}

func TestChecksumFor(t *testing.T) {
	const sums = `
9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08  claudestatus_linux_amd64
b1946ac92492d2347c6235b4d2611184b1946ac92492d2347c6235b4d2611184 *claudestatus_windows_amd64.exe
0000000000000000000000000000000000000000000000000000000000000000  claudestatus_darwin_arm64
`

	cases := []struct {
		asset string
		want  string
		found bool
	}{
		{"claudestatus_linux_amd64", "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", true},
		// The star marks a binary file in the sha256sum format and is not part
		// of the name.
		{"claudestatus_windows_amd64.exe", "b1946ac92492d2347c6235b4d2611184b1946ac92492d2347c6235b4d2611184", true},
		// The name has to match whole: a release with no binary for this
		// platform must not hand out somebody else's checksum.
		{"claudestatus_linux_arm64", "", false},
		{"claudestatus_linux", "", false},
		{"", "", false},
	}

	for _, c := range cases {
		got, found := checksumFor(sums, c.asset)
		if got != c.want || found != c.found {
			t.Errorf("checksumFor(%q) = %q, %t; want %q, %t", c.asset, got, found, c.want, c.found)
		}
	}
}

// The asset names are shared with install.sh and the release workflow: they can
// only be changed in all three places at once.
func TestAssetName(t *testing.T) {
	name := assetName()

	want := fmt.Sprintf("claudestatus_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if name != want {
		t.Errorf("assetName() = %q; want %q", name, want)
	}
}

func TestVersionFallsBackToDev(t *testing.T) {
	withVersion(t, "v1.2.3")
	if got := version(); got != "v1.2.3" {
		t.Errorf("version() = %q; want the release tag", got)
	}

	// A number that does not parse is no number at all — a hand-made build
	// counts as outdated rather than as version "dev-7".
	withVersion(t, "dev-7")
	if got := version(); got != devVersion {
		t.Errorf("version() = %q; want %q", got, devVersion)
	}
}

func TestCacheRoundTrip(t *testing.T) {
	testenv.Home(t)

	if _, ok := readCache(); ok {
		t.Error("an empty cache directory answered with a cache")
	}

	written := updateCache{CheckedAt: time.Now().Unix(), Latest: "v9.9.9"}
	if err := writeCache(written); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	read, ok := readCache()
	if !ok || read != written {
		t.Errorf("readCache() = %+v, %t; want %+v, true", read, ok, written)
	}

	// A cache that does not parse is no cache: the status line falls back to
	// drawing no update mark instead of failing.
	path, err := cachePath()
	if err != nil {
		t.Fatalf("cachePath: %v", err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatalf("damaging the cache: %v", err)
	}
	if _, ok := readCache(); ok {
		t.Error("a damaged cache was taken at face value")
	}
}

// A failed check only moves the timestamp: the known version stays, the mark
// does not go out, and the retries do not pile up every second.
func TestTouchCacheKeepsTheKnownVersion(t *testing.T) {
	testenv.Home(t)

	if err := writeCache(updateCache{CheckedAt: 1, Latest: "v9.9.9"}); err != nil {
		t.Fatalf("writeCache: %v", err)
	}
	touchCache()

	cache, ok := readCache()
	if !ok {
		t.Fatal("the cache disappeared")
	}
	if cache.Latest != "v9.9.9" {
		t.Errorf("Latest = %q; want the version to survive a failed check", cache.Latest)
	}
	if cache.CheckedAt == 1 {
		t.Error("CheckedAt did not move — the check would run again at once")
	}
}

func TestUpdateAvailable(t *testing.T) {
	cases := []struct {
		name      string
		installed string
		cached    string
		want      string
	}{
		{name: "a newer release is out", installed: "v1.0.0", cached: "v1.0.1", want: "v1.0.1"},
		{name: "the latest is installed", installed: "v1.0.1", cached: "v1.0.1"},
		{name: "the cache is behind", installed: "v1.0.2", cached: "v1.0.1"},
		// A build made outside a release never lights the mark up: there is
		// nothing to compare, and it cannot update itself into a known state.
		{name: "not a release build", installed: "", cached: "v9.9.9"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testenv.Home(t)
			withVersion(t, c.installed)

			if c.cached != "" {
				if err := writeCache(updateCache{CheckedAt: time.Now().Unix(), Latest: c.cached}); err != nil {
					t.Fatalf("writeCache: %v", err)
				}
			}

			tag, ok := updateAvailable()
			if ok != (c.want != "") || tag != c.want {
				t.Errorf("updateAvailable() = %q, %t; want %q, %t", tag, ok, c.want, c.want != "")
			}
		})
	}
}

func TestUpdateAvailableWithoutACache(t *testing.T) {
	testenv.Home(t)
	withVersion(t, "v1.0.0")

	if tag, ok := updateAvailable(); ok {
		t.Errorf("updateAvailable() = %q, true; want nothing before the first check", tag)
	}
}

func TestLatestVersion(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		want    string
		wantErr bool
	}{
		{name: "a release", status: http.StatusOK, body: `{"tag_name":"v1.2.3"}`, want: "v1.2.3"},
		{name: "no releases yet", status: http.StatusOK, body: `{}`, wantErr: true},
		{name: "not JSON", status: http.StatusOK, body: `<html>`, wantErr: true},
		{name: "GitHub is unhappy", status: http.StatusForbidden, body: `{"message":"rate limited"}`, wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer server.Close()
			withReleaseAPI(t, server.URL)

			got, err := latestVersion()
			if (err != nil) != c.wantErr {
				t.Fatalf("latestVersion() error = %v; wantErr %t", err, c.wantErr)
			}
			if got != c.want {
				t.Errorf("latestVersion() = %q; want %q", got, c.want)
			}
		})
	}
}

func TestReplaceSelf(t *testing.T) {
	const binary = "the new binary"
	releaseServer(t, map[string][]byte{assetName(): []byte(binary)})

	exe := stagedBinary(t, "the old binary")
	if err := replaceSelf(exe, "v9.9.9"); err != nil {
		t.Fatalf("replaceSelf: %v", err)
	}

	if got := readFile(t, exe); got != binary {
		t.Errorf("the binary holds %q; want %q", got, binary)
	}
	// Neither the staged file nor the previous binary is left behind.
	for _, leftover := range []string{exe + ".new", exe + ".old"} {
		if _, err := os.Stat(leftover); err == nil {
			t.Errorf("%s was left behind", filepath.Base(leftover))
		}
	}
}

// A binary whose checksum does not match is not installed: that is the whole
// point of downloading checksums.txt first.
func TestReplaceSelfRejectsABrokenDownload(t *testing.T) {
	asset := assetName()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			// The sum of something else entirely.
			fmt.Fprintf(w, "%s  %s\n", sha256Hex("what the release promised"), asset)
		default:
			_, _ = w.Write([]byte("what the release actually served"))
		}
	}))
	defer server.Close()
	withDownloadBase(t, server.URL+"/")

	exe := stagedBinary(t, "the old binary")
	err := replaceSelf(exe, "v9.9.9")
	if err == nil {
		t.Fatal("a binary with the wrong checksum was installed")
	}

	if got := readFile(t, exe); got != "the old binary" {
		t.Errorf("the running binary was replaced anyway: %q", got)
	}
	if _, err := os.Stat(exe + ".new"); err == nil {
		t.Error("the rejected download was left on disk")
	}
}

// A release built before this platform existed has no binary for it — and that
// has to be said, not guessed at.
func TestReplaceSelfWithoutAnAssetForThisPlatform(t *testing.T) {
	releaseServer(t, map[string][]byte{"claudestatus_plan9_mips": []byte("not ours")})

	exe := stagedBinary(t, "the old binary")
	if err := replaceSelf(exe, "v9.9.9"); err == nil {
		t.Fatal("a release without our binary went unnoticed")
	}
	if got := readFile(t, exe); got != "the old binary" {
		t.Errorf("the running binary was touched: %q", got)
	}
}

func TestCheckWritesTheCache(t *testing.T) {
	testenv.Home(t)
	withVersion(t, "v1.0.0")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	defer server.Close()
	withReleaseAPI(t, server.URL)

	output := testenv.CaptureStdout(t, func() {
		if err := check(false); err != nil {
			t.Errorf("check: %v", err)
		}
	})
	if !strings.Contains(output, "v9.9.9") {
		t.Errorf("check said %q; want the new version in it", output)
	}

	cache, ok := readCache()
	if !ok || cache.Latest != "v9.9.9" {
		t.Fatalf("readCache() = %+v, %t; want v9.9.9", cache, ok)
	}
	// The status line reads that cache and nothing else.
	if tag, ok := updateAvailable(); !ok || tag != "v9.9.9" {
		t.Errorf("updateAvailable() = %q, %t; want v9.9.9, true", tag, ok)
	}
}

// A check that could not reach GitHub leaves the known version alone: the mark
// must not go out because the network blinked.
func TestFailedCheckKeepsTheMark(t *testing.T) {
	testenv.Home(t)
	withVersion(t, "v1.0.0")
	if err := writeCache(updateCache{CheckedAt: 1, Latest: "v9.9.9"}); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	withReleaseAPI(t, server.URL)

	if err := check(true); err == nil {
		t.Fatal("a failed check reported success")
	}
	if tag, ok := updateAvailable(); !ok || tag != "v9.9.9" {
		t.Errorf("updateAvailable() = %q, %t; want the known version to survive", tag, ok)
	}
}

// releaseServer serves checksums.txt and the assets of a release, as GitHub
// does, and points downloadBase at itself.
func releaseServer(t *testing.T, assets map[string][]byte) {
	t.Helper()

	var sums strings.Builder
	for name, content := range assets {
		fmt.Fprintf(&sums, "%s  %s\n", sha256Hex(string(content)), name)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		if name == "checksums.txt" {
			_, _ = w.Write([]byte(sums.String()))
			return
		}
		content, ok := assets[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(content)
	}))
	t.Cleanup(server.Close)
	withDownloadBase(t, server.URL+"/")
}

// stagedBinary stands in for the running binary: replaceSelf renames files
// around it, so it needs a directory of its own.
func stagedBinary(t *testing.T, content string) string {
	t.Helper()

	exe := filepath.Join(t.TempDir(), "claudestatus")
	if err := os.WriteFile(exe, []byte(content), 0o755); err != nil {
		t.Fatalf("writing the binary: %v", err)
	}
	return exe
}

func withReleaseAPI(t *testing.T, url string) {
	t.Helper()

	saved := releaseAPI
	releaseAPI = url
	t.Cleanup(func() { releaseAPI = saved })
}

func withDownloadBase(t *testing.T, url string) {
	t.Helper()

	saved := downloadBase
	downloadBase = url
	t.Cleanup(func() { downloadBase = saved })
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}
