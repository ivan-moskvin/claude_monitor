package main

import (
	"context"
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
)

// Обновляемся тем же способом, каким ставились: go install нужного тега.
// Клона на диске нет — версии берём у Go-прокси, он же отдаёт исходники.
const (
	modulePath = "github.com/ivan-moskvin/claude_monitor"
	// Первый вызов проверяет сразу — кэша ещё нет, — дальше не чаще раза в час.
	updateCheckInterval = time.Hour
	networkTimeout      = 20 * time.Second
	installTimeout      = 5 * time.Minute
)

// updateCache лежит в кэше пользователя: это не настройка, потерять его не жаль.
type updateCache struct {
	CheckedAt int64  `json:"checked_at"`
	Latest    string `json:"latest"`
}

// check спрашивает у прокси последнюю версию модуля и запоминает её.
// С --quiet работает молча — так её зовёт строка статуса.
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
		fmt.Printf("Установлено %s, вышло %s — обновиться: claudestatus update\n", version(), latest)
	case version() == devVersion:
		fmt.Printf("Сборка не из тега, последняя версия — %s\n", latest)
	default:
		fmt.Printf("Установлена последняя версия: %s\n", version())
	}
	return nil
}

// update переустанавливает утилиту последним тегом и заново прописывает строку
// статуса — путь бинаря при этом не меняется, но настройки могут быть чужими.
func update() error {
	latest, err := latestVersion()
	if err != nil {
		return err
	}
	_ = writeCache(updateCache{CheckedAt: time.Now().Unix(), Latest: latest})

	if !newer(version(), latest) && version() != devVersion {
		fmt.Printf("Уже последняя версия: %s\n", version())
		return nil
	}

	if version() == devVersion {
		fmt.Printf("==> Установка %s\n", latest)
	} else {
		fmt.Printf("==> Обновление %s → %s\n", version(), latest)
	}
	if err := goInstall(latest); err != nil {
		return err
	}

	// В настройки пишем тот бинарь, который только что положил go install:
	// запущенный сейчас может быть и сборкой из исходников в другом каталоге.
	installed, err := installedPath()
	if err != nil {
		return err
	}
	if err := install(installed); err != nil {
		return err
	}

	fmt.Printf("\nГотово: %s. Строка статуса обновится в следующей сессии Claude Code.\n", latest)
	return nil
}

// goInstall ставит нужную версию поверх текущей. Запущенный бинарь при этом
// подменяется целиком — go пишет новый файл рядом и переименовывает.
func goInstall(tag string) error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("не нашёлся go — поставьте его (brew install go) и повторите")
	}

	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()

	target := modulePath + "/claudestatus@" + tag
	cmd := exec.CommandContext(ctx, "go", "install", target)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go install %s: %w", target, err)
	}
	return nil
}

// installedPath — куда go install положил бинарь: GOBIN, а если он не задан,
// то первый GOPATH/bin.
func installedPath() (string, error) {
	out, err := exec.Command("go", "env", "GOBIN", "GOPATH").Output()
	if err != nil {
		return "", fmt.Errorf("не удалось спросить у go, куда он ставит бинари: %w", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 2 {
		return "", fmt.Errorf("неожиданный ответ go env: %q", string(out))
	}

	dir := strings.TrimSpace(lines[0])
	if dir == "" {
		paths := filepath.SplitList(strings.TrimSpace(lines[1]))
		if len(paths) == 0 || paths[0] == "" {
			return "", fmt.Errorf("go не сказал ни GOBIN, ни GOPATH")
		}
		dir = filepath.Join(paths[0], "bin")
	}

	name := "claudestatus"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, name), nil
}

// latestVersion спрашивает версию у Go-прокси — того же, через который утилита
// ставится. Тег в репозитории и версия модуля здесь одно и то же.
func latestVersion() (string, error) {
	proxy := strings.TrimSuffix(firstProxy(), "/")
	url := fmt.Sprintf("%s/%s/@latest", proxy, modulePath)

	ctx, cancel := context.WithTimeout(context.Background(), networkTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("не удалось узнать последнюю версию: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return "", fmt.Errorf("прокси ответил %s: %s", response.Status, strings.TrimSpace(string(body)))
	}

	var info struct {
		Version string `json:"Version"`
	}
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("не удалось разобрать ответ прокси: %w", err)
	}
	if info.Version == "" {
		return "", fmt.Errorf("прокси не знает версий %s — тег ещё не опубликован", modulePath)
	}
	return info.Version, nil
}

// firstProxy уважает GOPROXY: кто настроил себе зеркало, к нему и пойдёт.
// direct и off здесь не годятся — по HTTP спрашивать некого.
func firstProxy() string {
	const public = "https://proxy.golang.org"

	for _, entry := range strings.FieldsFunc(os.Getenv("GOPROXY"), func(r rune) bool { return r == ',' || r == '|' }) {
		entry = strings.TrimSpace(entry)
		if strings.HasPrefix(entry, "http://") || strings.HasPrefix(entry, "https://") {
			return entry
		}
	}
	return public
}

// updateAvailable отвечает по кэшу — строка статуса в сеть не ходит.
func updateAvailable() (string, bool) {
	cache, ok := readCache()
	if !ok || !newer(version(), cache.Latest) {
		return "", false
	}
	return cache.Latest, true
}

// autoCheck запускает проверку отдельным процессом — при первом вызове и потом
// не чаще раза в час, то есть и в начале сессии, и по ходу работы. Ждать его
// нельзя: строка статуса рисуется на каждый чих и должна возвращаться сразу.
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

	// Отметку времени ставим до запуска: иначе несколько сессий разом
	// поднимут по своей проверке.
	touchCache()

	cmd := exec.Command(exe, "check", "--quiet")
	// Вывод отвязываем от нашего: Claude Code читает stdout строки статуса
	// и ждал бы закрытия трубы фоновым процессом.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if cmd.Start() == nil {
		_ = cmd.Process.Release()
	}
}

func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "claudestatus", "update.json"), nil
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
	// Пишем через временный файл: строка статуса может читать кэш прямо сейчас.
	staged := path + ".tmp"
	if err := os.WriteFile(staged, data, 0o644); err != nil {
		return err
	}
	return os.Rename(staged, path)
}

// touchCache отмечает попытку проверки, не трогая известную версию: неудачный
// запрос не должен ни гасить значок обновления, ни звать проверку каждую секунду.
func touchCache() {
	cache, _ := readCache()
	cache.CheckedAt = time.Now().Unix()
	_ = writeCache(cache)
}

// versionOverride задаётся при ручной сборке (-X main.versionOverride=v1.2.3):
// у собранного локально бинаря версии нет, а демо и проверки её требуют.
var versionOverride string

// version читает версию из самого бинаря: go install проставляет её сам,
// собирать с -ldflags ради этого не нужно.
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

// parseVersion читает и голый тег, и псевдоверсию Go: суффикс после дефиса
// (-rc1, -0.20240101-abcdef) на сравнение номера не влияет.
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

// newer сравнивает установленную версию с тегом. Сборка не из тега считается
// устаревшей: у неё нет номера, с которым можно сравнивать.
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
