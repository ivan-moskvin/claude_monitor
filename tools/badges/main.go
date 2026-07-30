// Command badges draws the README badges into .github/badges.
//
// Shields.io would draw them too, but every badge would then be a request from
// the reader's browser to somebody else's server, and the numbers about the
// tests would have to be published somewhere first. These are plain files in
// the repository: they show the state of the commit they were made in, and they
// keep working with no network at all.
//
// Run it after touching the tests or the release matrix:
//
//	go run ./tools/badges
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const badgesDir = ".github/badges"

// The colors are the ones shields.io uses, so that the badges do not look like
// strangers next to the ones people are used to. Go keeps its own brand blue.
const (
	colorGreen  = "#4c1"
	colorBlue   = "#007ec6"
	colorGo     = "#00ADD8"
	colorRed    = "#e05d44"
	colorLabel  = "#555"
	colorYellow = "#dfb317"
	colorOrange = "#fe7d37"
	colorLime   = "#97ca00"
	colorOlive  = "#a4a61d"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, badgesDir), 0o755); err != nil {
		return err
	}

	fmt.Println("running the tests…")
	result, err := runTests(root)
	if err != nil {
		return err
	}

	badges := map[string]string{
		"tests.svg":    testsBadge(result),
		"coverage.svg": flatBadge("coverage", fmt.Sprintf("%.1f%%", result.coverage), coverageColor(result.coverage)),
	}

	version, err := goVersion(root)
	if err != nil {
		return err
	}
	badges["go.svg"] = flatBadge("Go", version, colorGo)

	targets, err := platforms(root)
	if err != nil {
		return err
	}
	badges["platforms.svg"] = flatBadge("platforms", strings.Join(targets, " · "), colorBlue)

	// No dependencies is a promise, not a coincidence: the utility downloads a
	// single binary and has nothing else to keep up to date.
	direct, err := directDependencies(root)
	if err != nil {
		return err
	}
	if direct == 0 {
		badges["dependencies.svg"] = flatBadge("dependencies", "zero", colorGreen)
	} else {
		badges["dependencies.svg"] = flatBadge("dependencies", strconv.Itoa(direct), colorBlue)
	}

	for name, svg := range badges {
		path := filepath.Join(root, badgesDir, name)
		if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
			return err
		}
	}

	fmt.Printf("%d badges written to %s\n", len(badges), badgesDir)
	fmt.Printf("  tests: %d passed", result.passed)
	if result.failed > 0 {
		fmt.Printf(", %d failed", result.failed)
	}
	fmt.Printf(", coverage %.1f%%\n", result.coverage)
	return nil
}

type testResult struct {
	passed   int
	failed   int
	coverage float64
}

// runTests counts the tests and measures the coverage in one pass: -json gives
// an event per test, the profile gives the percentage over every package —
// including the ones that have no tests of their own, which is the honest
// number for a utility whose other half only talks to the network.
func runTests(root string) (testResult, error) {
	profile := filepath.Join(os.TempDir(), "claudestatus-coverage.out")
	defer os.Remove(profile)

	packages, err := shippedPackages(root)
	if err != nil {
		return testResult{}, err
	}

	arguments := append([]string{"test"}, packages...)
	arguments = append(arguments, "-json",
		"-coverprofile="+profile, "-covermode=count", "-coverpkg="+strings.Join(packages, ","))

	test := exec.Command("go", arguments...)
	test.Dir = root
	test.Stderr = os.Stderr

	output, err := test.Output()
	// A failing test is not a reason to stop: the badge is there to say so.
	if err != nil && len(output) == 0 {
		return testResult{}, fmt.Errorf("go test: %w", err)
	}

	var result testResult
	for _, line := range strings.Split(string(output), "\n") {
		if line == "" {
			continue
		}
		var event struct {
			Action string `json:"Action"`
			Test   string `json:"Test"`
		}
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		// Without a Test name the event is about a whole package.
		if event.Test == "" {
			continue
		}
		switch event.Action {
		case "pass":
			result.passed++
		case "fail":
			result.failed++
		}
	}

	result.coverage, err = totalCoverage(root, profile)
	if err != nil {
		return result, err
	}
	return result, nil
}

// shippedPackages — everything that goes into the binary. The generator itself
// and the test helpers are not part of the utility and would only water the
// coverage down.
func shippedPackages(root string) ([]string, error) {
	list := exec.Command("go", "list", "./...")
	list.Dir = root

	output, err := list.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}

	var packages []string
	for _, name := range strings.Fields(string(output)) {
		if strings.Contains(name, "/tools/") || strings.HasSuffix(name, "/internal/testenv") {
			continue
		}
		packages = append(packages, name)
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("go list found no packages")
	}
	return packages, nil
}

var coverageTotal = regexp.MustCompile(`total:\s+\(statements\)\s+([0-9.]+)%`)

func totalCoverage(root, profile string) (float64, error) {
	report := exec.Command("go", "tool", "cover", "-func="+profile)
	report.Dir = root

	output, err := report.Output()
	if err != nil {
		return 0, fmt.Errorf("go tool cover: %w", err)
	}
	match := coverageTotal.FindStringSubmatch(string(output))
	if match == nil {
		return 0, fmt.Errorf("go tool cover printed no total")
	}
	return strconv.ParseFloat(match[1], 64)
}

func testsBadge(result testResult) string {
	if result.failed > 0 {
		return flatBadge("tests", fmt.Sprintf("%d failed, %d passed", result.failed, result.passed), colorRed)
	}
	return flatBadge("tests", fmt.Sprintf("%d passed", result.passed), colorGreen)
}

func coverageColor(percentage float64) string {
	switch {
	case percentage >= 90:
		return colorGreen
	case percentage >= 80:
		return colorLime
	case percentage >= 70:
		return colorOlive
	case percentage >= 60:
		return colorYellow
	case percentage >= 50:
		return colorOrange
	default:
		return colorRed
	}
}

var goDirective = regexp.MustCompile(`(?m)^go\s+([0-9]+\.[0-9]+)`)

func goVersion(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	match := goDirective.FindSubmatch(data)
	if match == nil {
		return "", fmt.Errorf("go.mod names no Go version")
	}
	return string(match[1]), nil
}

// directDependencies counts what go.mod requires: a require block, or a single
// require line.
var requireLine = regexp.MustCompile(`(?m)^\s*require\s+\(([^)]*)\)|^\s*require\s+(\S+)\s`)

func directDependencies(root string) (int, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return 0, err
	}

	count := 0
	for _, match := range requireLine.FindAllStringSubmatch(string(data), -1) {
		if match[2] != "" {
			count++
			continue
		}
		for _, line := range strings.Split(match[1], "\n") {
			if strings.TrimSpace(line) != "" {
				count++
			}
		}
	}
	return count, nil
}

// platforms reads the release matrix out of the workflow: the badge promises
// exactly the systems a release is actually built for, and cannot drift away
// from them.
var releaseTargets = regexp.MustCompile(`for target in ([^;]+);`)

func platforms(root string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/release.yml"))
	if err != nil {
		return nil, err
	}
	match := releaseTargets.FindSubmatch(data)
	if match == nil {
		return nil, fmt.Errorf("release.yml names no build targets")
	}

	names := map[string]string{"darwin": "macOS", "linux": "Linux", "windows": "Windows"}
	var found []string
	seen := map[string]bool{}
	for _, target := range strings.Fields(string(match[1])) {
		goos, _, ok := strings.Cut(target, "/")
		if !ok {
			continue
		}
		name, known := names[goos]
		if !known {
			name = goos
		}
		if !seen[name] {
			seen[name] = true
			found = append(found, name)
		}
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("release.yml names no build targets")
	}
	return found, nil
}

func repositoryRoot() (string, error) {
	// The generator lives in tools/badges and is run with `go run ./tools/badges`
	// from anywhere inside the repository.
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("this is not a git repository: %w", err)
	}
	return strings.TrimSpace(string(root)), nil
}

// The SVG below is the shields.io "flat" badge, drawn by hand: a grey label, a
// colored message, one gradient over both. The width is guessed from the
// characters, because there is no font to measure with.
func charWidth(r rune) float64 {
	switch {
	case strings.ContainsRune("il.:,|'!;[]()", r):
		return 3.2
	case strings.ContainsRune("ftrj/\\ ", r):
		return 4.2
	case strings.ContainsRune("mwMW@%·", r):
		return 10.5
	case r >= 'A' && r <= 'Z':
		return 8.2
	default:
		return 6.8
	}
}

func textWidth(text string) float64 {
	var width float64
	for _, r := range text {
		width += charWidth(r)
	}
	return width
}

func flatBadge(label, message, color string) string {
	const padding = 12

	labelWidth := int(textWidth(label) + padding + 0.5)
	messageWidth := int(textWidth(message) + padding + 0.5)
	width := labelWidth + messageWidth

	// The text is drawn at ten times the size and scaled down, the way
	// shields.io does it: that is what keeps the letters crisp.
	labelX := labelWidth * 10 / 2
	messageX := (labelWidth + messageWidth/2) * 10
	labelLength := max(0, (labelWidth-padding)*10)
	messageLength := max(0, (messageWidth-padding)*10)

	title := label + ": " + message
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="%d" height="20" role="img" aria-label="%s">
  <title>%s</title>
  <linearGradient id="s" x2="0" y2="100%%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>
  <clipPath id="r"><rect width="%d" height="20" rx="3" fill="#fff"/></clipPath>
  <g clip-path="url(#r)">
    <rect width="%d" height="20" fill="%s"/>
    <rect x="%d" width="%d" height="20" fill="%s"/>
    <rect width="%d" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" text-rendering="geometricPrecision" font-size="110">
    <text aria-hidden="true" x="%d" y="150" fill="#010101" fill-opacity=".3" transform="scale(.1)" textLength="%d">%s</text>
    <text x="%d" y="140" transform="scale(.1)" textLength="%d">%s</text>
    <text aria-hidden="true" x="%d" y="150" fill="#010101" fill-opacity=".3" transform="scale(.1)" textLength="%d">%s</text>
    <text x="%d" y="140" transform="scale(.1)" textLength="%d">%s</text>
  </g>
</svg>
`,
		width, title, title,
		width,
		labelWidth, colorLabel,
		labelWidth, messageWidth, color,
		width,
		labelX, labelLength, label,
		labelX, labelLength, label,
		messageX, messageLength, message,
		messageX, messageLength, message)
}
