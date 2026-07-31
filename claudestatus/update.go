package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/ivan-moskvin/claudestatus/i18n"
)

// We update the same way we were installed: download the ready binary from a
// release and replace ourselves with it. Neither Go nor a clone of the
// repository is needed on the machine.
const (
	repository = "ivan-moskvin/claudestatus"
	// The first call checks right away — there is no cache yet — and after that
	// no more than once an hour.
	updateCheckInterval = time.Hour
	networkTimeout      = 20 * time.Second
	downloadTimeout     = 5 * time.Minute
)

// The addresses are variables so that updating can be tested against a local
// server without publishing a release.
var (
	releaseAPI   = "https://api.github.com/repos/" + repository + "/releases/latest"
	downloadBase = "https://github.com/" + repository + "/releases/download/"
)

// updateCache lives in the user cache: it is not a setting, losing it costs
// nothing.
type updateCache struct {
	CheckedAt int64  `json:"checked_at"`
	Latest    string `json:"latest"`
}

// check asks GitHub for the latest release and remembers its version.
// With --quiet it works silently — that is how the status line calls it.
func check(quiet bool) error {
	touchCache()

	latest, err := latestVersion()
	if err != nil {
		return err
	}
	if err := writeCache(updateCache{CheckedAt: time.Now().Unix(), Latest: latest}); err != nil {
		return err
	}

	if quiet {
		return nil
	}

	switch {
	case newer(version(), latest):
		fmt.Printf(i18n.T("%s installed, %s is out — update with: claudestatus update\n"), version(), latest)
	case version() == devVersion:
		fmt.Printf(i18n.T("Not a release build, the latest version is %s\n"), latest)
	default:
		fmt.Printf(i18n.T("The latest version is installed: %s\n"), version())
	}
	return nil
}

// update downloads the binary of the latest release and replaces itself with it.
func update() error {
	latest, err := latestVersion()
	if err != nil {
		return err
	}
	_ = writeCache(updateCache{CheckedAt: time.Now().Unix(), Latest: latest})

	if !newer(version(), latest) && version() != devVersion {
		fmt.Printf(i18n.T("Already the latest version: %s\n"), version())
		return nil
	}

	exe, err := selfPath()
	if err != nil {
		return err
	}

	if version() == devVersion {
		fmt.Printf(i18n.T("==> Installing %s\n"), latest)
	} else {
		fmt.Printf(i18n.T("==> Updating %s → %s\n"), version(), latest)
	}
	if err := replaceSelf(exe, latest); err != nil {
		return err
	}

	fmt.Printf(i18n.T("Done: %s. The status line picks it up by itself, no need to restart Claude Code.\n"), latest)
	return nil
}

// replaceSelf downloads the release binary next to the current one and swaps
// them by renaming: writing over a running file is not allowed, while a rename
// survives even our own running process.
func replaceSelf(exe, tag string) error {
	asset := assetName()

	sums, err := download(downloadBase + tag + "/checksums.txt")
	if err != nil {
		return fmt.Errorf(i18n.T("could not download the checksums: %w"), err)
	}
	want, ok := checksumFor(string(sums), asset)
	if !ok {
		return fmt.Errorf(i18n.T("release %s has no binary for %s/%s"), tag, runtime.GOOS, runtime.GOARCH)
	}

	binary, err := download(downloadBase + tag + "/" + asset)
	if err != nil {
		return fmt.Errorf(i18n.T("could not download %s: %w"), asset, err)
	}

	got := sha256.Sum256(binary)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf(i18n.T("the checksum of %s does not match — the file is broken or tampered with"), asset)
	}

	staged := exe + ".new"
	if err := os.WriteFile(staged, binary, 0o755); err != nil {
		return fmt.Errorf(i18n.T("could not write %s: %w"), staged, err)
	}

	previous := exe + ".old"
	_ = os.Remove(previous)
	if err := os.Rename(exe, previous); err != nil {
		_ = os.Remove(staged)
		return fmt.Errorf(i18n.T("could not move the previous binary away: %w"), err)
	}
	if err := os.Rename(staged, exe); err != nil {
		_ = os.Rename(previous, exe)
		return fmt.Errorf(i18n.T("could not put the new binary in place: %w"), err)
	}
	// Windows will not let a running file be deleted — it goes away on the next
	// update.
	_ = os.Remove(previous)
	return nil
}

// assetName — the file name in the release for the current platform.
func assetName() string {
	name := fmt.Sprintf("claudestatus_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// checksumFor looks for a line of the form "<sha256>  <file>" — the sha256sum
// format.
func checksumFor(sums, asset string) (string, bool) {
	for line := range strings.SplitSeq(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return fields[0], true
		}
	}
	return "", false
}

// latestVersion asks GitHub for the tag of the latest release.
func latestVersion() (string, error) {
	body, err := fetch(releaseAPI, networkTimeout, "application/vnd.github+json")
	if err != nil {
		return "", fmt.Errorf(i18n.T("could not find out the latest version: %w"), err)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf(i18n.T("could not parse the GitHub response: %w"), err)
	}
	if release.TagName == "" {
		return "", fmt.Errorf(i18n.T("%s has no releases yet"), repository)
	}
	return release.TagName, nil
}

func download(url string) ([]byte, error) {
	return fetch(url, downloadTimeout, "application/octet-stream")
}

func fetch(url string, timeout time.Duration, accept string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", accept)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(i18n.T("%s answered %s"), url, response.Status)
	}
	return io.ReadAll(response.Body)
}

// updateAvailable answers from the cache — the status line never goes to the
// network.
func updateAvailable() (string, bool) {
	cache, ok := readCache()
	if !ok || !newer(version(), cache.Latest) {
		return "", false
	}
	return cache.Latest, true
}

// autoCheck starts the check as a separate process — on the first call and then
// no more than once an hour, that is both at the start of a session and while
// working. Waiting for it is not allowed: the status line is drawn on every
// keystroke and has to return at once.
func autoCheck() {
	if os.Getenv("CLAUDESTATUS_NO_AUTO_UPDATE") != "" {
		return
	}
	if cache, ok := readCache(); ok && time.Since(time.Unix(cache.CheckedAt, 0)) < updateCheckInterval {
		return
	}
	exe, err := selfPath()
	if err != nil {
		return
	}

	// The timestamp is written before starting: otherwise several sessions
	// would each spawn their own check.
	touchCache()

	cmd := exec.Command(exe, "check", "--quiet")
	// The output is detached from ours: Claude Code reads the stdout of the
	// status line and would wait for the background process to close the pipe.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if cmd.Start() == nil {
		_ = cmd.Process.Release()
	}
}

func cacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "claudestatus"), nil
}

func cachePath() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "update.json"), nil
}

func readCache() (updateCache, bool) {
	path, err := cachePath()
	if err != nil {
		return updateCache{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return updateCache{}, false
	}
	var cache updateCache
	if json.Unmarshal(data, &cache) != nil {
		return updateCache{}, false
	}
	return cache, true
}

func writeCache(cache updateCache) error {
	path, err := cachePath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Written through a temporary file: the status line may be reading the
	// cache right now.
	staged := path + ".tmp"
	if err := os.WriteFile(staged, data, 0o644); err != nil {
		return err
	}
	return os.Rename(staged, path)
}

// touchCache records an attempt to check without touching the known version: a
// failed request must neither put out the update mark nor make the check run
// every second.
func touchCache() {
	cache, _ := readCache()
	cache.CheckedAt = time.Now().Unix()
	_ = writeCache(cache)
}

// versionOverride is set when a release is built (-X main.versionOverride=v1.2.3).
var versionOverride string

// version reads the version out of the binary itself: CI stamps it into a
// release build, a hand-made build has none — such a build counts as outdated
// and never lights the mark up.
func version() string {
	if _, valid := parseVersion(versionOverride); valid {
		return versionOverride
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return devVersion
	}
	if _, valid := parseVersion(info.Main.Version); !valid {
		return devVersion
	}
	return info.Main.Version
}

type semver struct{ major, minor, patch int }

func (v semver) less(other semver) bool {
	if v.major != other.major {
		return v.major < other.major
	}
	if v.minor != other.minor {
		return v.minor < other.minor
	}
	return v.patch < other.patch
}

// parseVersion reads a tag of the form v1.2.3: a suffix after the dash (-rc1)
// does not take part in comparing the number.
func parseVersion(value string) (semver, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	if i := strings.IndexAny(value, "-+"); i >= 0 {
		value = value[:i]
	}

	fields := strings.Split(value, ".")
	if len(fields) != 3 {
		return semver{}, false
	}

	var parsed semver
	for i, target := range []*int{&parsed.major, &parsed.minor, &parsed.patch} {
		number, err := strconv.Atoi(fields[i])
		if err != nil || number < 0 {
			return semver{}, false
		}
		*target = number
	}
	return parsed, true
}

// newer compares the installed version against a tag. A build made outside a
// release counts as outdated: it has no number to compare with.
func newer(current, tag string) bool {
	latest, ok := parseVersion(tag)
	if !ok {
		return false
	}
	installed, ok := parseVersion(current)
	if !ok {
		return false
	}
	return installed.less(latest)
}
