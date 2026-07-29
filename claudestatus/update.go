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
)

// Обновляемся тем же способом, каким ставились: качаем готовый бинарь из релиза
// и подменяем им себя. Ни go, ни клона репозитория на машине не нужно.
const (
	repository = "ivan-moskvin/claude_monitor"
	// Первый вызов проверяет сразу — кэша ещё нет, — дальше не чаще раза в час.
	updateCheckInterval = time.Hour
	networkTimeout      = 20 * time.Second
	downloadTimeout     = 5 * time.Minute
)

// Адреса вынесены в переменные, чтобы проверять обновление на локальном сервере,
// не выкладывая релиз.
var (
	releaseAPI   = "https://api.github.com/repos/" + repository + "/releases/latest"
	downloadBase = "https://github.com/" + repository + "/releases/download/"
)

// updateCache лежит в кэше пользователя: это не настройка, потерять его не жаль.
type updateCache struct {
	CheckedAt int64  `json:"checked_at"`
	Latest    string `json:"latest"`
}

// check спрашивает у GitHub последний релиз и запоминает его версию.
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
		fmt.Printf("Сборка не из релиза, последняя версия — %s\n", latest)
	default:
		fmt.Printf("Установлена последняя версия: %s\n", version())
	}
	return nil
}

// update качает бинарь последнего релиза и заменяет им себя.
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

	exe, err := selfPath()
	if err != nil {
		return err
	}

	if version() == devVersion {
		fmt.Printf("==> Установка %s\n", latest)
	} else {
		fmt.Printf("==> Обновление %s → %s\n", version(), latest)
	}
	if err := replaceSelf(exe, latest); err != nil {
		return err
	}

	fmt.Printf("Готово: %s. Строка статуса обновится сама, перезапускать Claude Code не нужно.\n", latest)
	return nil
}

// replaceSelf скачивает бинарь релиза рядом с текущим и подменяет его
// переименованием: писать поверх работающего файла нельзя, а rename переживает
// даже собственный запущенный процесс.
func replaceSelf(exe, tag string) error {
	asset := assetName()

	sums, err := download(downloadBase + tag + "/checksums.txt")
	if err != nil {
		return fmt.Errorf("не удалось скачать контрольные суммы: %w", err)
	}
	want, ok := checksumFor(string(sums), asset)
	if !ok {
		return fmt.Errorf("в релизе %s нет бинаря для %s/%s", tag, runtime.GOOS, runtime.GOARCH)
	}

	binary, err := download(downloadBase + tag + "/" + asset)
	if err != nil {
		return fmt.Errorf("не удалось скачать %s: %w", asset, err)
	}

	got := sha256.Sum256(binary)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("контрольная сумма %s не сошлась — файл побился или подменён", asset)
	}

	staged := exe + ".new"
	if err := os.WriteFile(staged, binary, 0o755); err != nil {
		return fmt.Errorf("не удалось записать %s: %w", staged, err)
	}

	previous := exe + ".old"
	_ = os.Remove(previous)
	if err := os.Rename(exe, previous); err != nil {
		_ = os.Remove(staged)
		return fmt.Errorf("не удалось убрать прежний бинарь: %w", err)
	}
	if err := os.Rename(staged, exe); err != nil {
		_ = os.Rename(previous, exe)
		return fmt.Errorf("не удалось поставить новый бинарь: %w", err)
	}
	// В Windows запущенный файл удалить не дадут — уберётся при следующем update.
	_ = os.Remove(previous)
	return nil
}

// assetName — имя файла в релизе для текущей платформы.
func assetName() string {
	name := fmt.Sprintf("claudestatus_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// checksumFor ищет строку вида "<sha256>  <файл>" — формат sha256sum.
func checksumFor(sums, asset string) (string, bool) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return fields[0], true
		}
	}
	return "", false
}

// latestVersion спрашивает у GitHub тег последнего релиза.
func latestVersion() (string, error) {
	body, err := fetch(releaseAPI, networkTimeout, "application/vnd.github+json")
	if err != nil {
		return "", fmt.Errorf("не удалось узнать последнюю версию: %w", err)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("не удалось разобрать ответ GitHub: %w", err)
	}
	if release.TagName == "" {
		return "", fmt.Errorf("у %s ещё нет релизов", repository)
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
		return nil, fmt.Errorf("%s ответил %s", url, response.Status)
	}
	return io.ReadAll(response.Body)
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

// versionOverride проставляется при сборке релиза (-X main.versionOverride=v1.2.3).
var versionOverride string

// version читает версию из самого бинаря: у релизной сборки её проставляет CI,
// у собранной руками её нет — такая считается устаревшей и значок не зажигает.
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

// parseVersion читает тег вида v1.2.3: суффикс после дефиса (-rc1) на сравнение
// номера не влияет.
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

// newer сравнивает установленную версию с тегом. Сборка не из релиза считается
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
