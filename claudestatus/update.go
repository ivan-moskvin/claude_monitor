package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Обновляемся по тегам: репозиторий — это и есть канал доставки, релизных
// артефактов у утилиты нет. Промежуточные коммиты в main обновлением не считаем.
const (
	updateCheckInterval = 24 * time.Hour
	fetchTimeout        = 30 * time.Second
	sourceDir           = "claudestatus"
)

// updateCache лежит в bin/ рядом с бинарём: он такой же артефакт сборки,
// уезжает и удаляется вместе с клоном.
type updateCache struct {
	CheckedAt int64  `json:"checked_at"`
	Latest    string `json:"latest"`
}

// check спрашивает у origin список тегов и запоминает самый свежий.
// С --quiet работает молча — так её зовёт строка статуса.
func check(quiet bool) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}

	touchCache(root)

	tag, err := latestTag(root)
	if err != nil {
		return err
	}
	if err := writeCache(root, updateCache{CheckedAt: time.Now().Unix(), Latest: tag}); err != nil {
		return err
	}

	if quiet {
		return nil
	}

	switch {
	case newer(version, tag):
		fmt.Printf("Установлено %s, вышло %s — обновиться: claudestatus update\n", version, tag)
	case version == devVersion:
		fmt.Printf("Сборка не из тега (%s), последний тег — %s\n", version, tag)
	default:
		fmt.Printf("Установлена последняя версия: %s\n", version)
	}
	return nil
}

// update подтягивает последний тег, пересобирает бинарь и переустанавливает
// строку статуса. Всё происходит в клоне — другого места установки нет.
func update() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}

	if dirty, err := gitOutput(root, "status", "--porcelain"); err != nil {
		return err
	} else if dirty != "" {
		return fmt.Errorf("в %s есть незакоммиченные изменения — обновление их затрёт", root)
	}

	tag, err := latestTag(root)
	if err != nil {
		return err
	}
	_ = writeCache(root, updateCache{CheckedAt: time.Now().Unix(), Latest: tag})

	if !newer(version, tag) && version != devVersion {
		fmt.Printf("Уже последняя версия: %s\n", version)
		return nil
	}

	fmt.Printf("==> Обновление %s → %s\n", version, tag)
	if err := checkout(root, tag); err != nil {
		return err
	}

	fmt.Println("==> Сборка")
	if err := rebuild(root, tag); err != nil {
		return err
	}

	fmt.Println("==> Установка")
	if err := install(); err != nil {
		return err
	}

	fmt.Printf("\nГотово: %s. Строка статуса обновится в следующей сессии Claude Code.\n", tag)
	return nil
}

// checkout старается остаться на ветке: перемотка вперёд сохраняет привычное
// состояние клона, а detached HEAD берём только если ветка ушла в сторону.
func checkout(root, tag string) error {
	if branch, err := gitOutput(root, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil && branch != "" {
		if _, err := gitOutput(root, "merge", "--ff-only", tag); err == nil {
			return nil
		}
	}
	_, err := gitOutput(root, "-c", "advice.detachedHead=false", "checkout", "--quiet", tag)
	return err
}

// rebuild собирает новый бинарь рядом и подменяет им текущий: писать поверх
// работающего файла нельзя, а переименование переживает даже запущенный процесс.
func rebuild(root, tag string) error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("не нашёлся go — соберите через ./run.sh (Windows: .\\run.ps1)")
	}

	exe, err := selfPath()
	if err != nil {
		return err
	}

	source := filepath.Join(root, sourceDir)
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("не нашлись исходники в %s — соберите через ./run.sh", source)
	}

	staged := exe + ".new"
	cmd := exec.Command("go", "build", "-trimpath",
		"-ldflags", "-s -w -X main.version="+tag, "-o", staged, ".")
	cmd.Dir = source
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("сборка не удалась: %w", err)
	}

	previous := exe + ".old"
	_ = os.Remove(previous)
	if err := os.Rename(exe, previous); err != nil {
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

// latestTag тянет теги с origin и возвращает старший по номеру версии.
func latestTag(root string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("не нашёлся git — без него обновляться неоткуда")
	}
	if _, err := gitOutput(root, "fetch", "--tags", "--force", "--quiet", "origin"); err != nil {
		return "", fmt.Errorf("не удалось получить теги: %w", err)
	}

	out, err := gitOutput(root, "tag", "--list")
	if err != nil {
		return "", err
	}

	best, found := semver{}, ""
	for _, line := range strings.Fields(out) {
		parsed, ok := parseVersion(line)
		if !ok {
			continue
		}
		if found == "" || best.less(parsed) {
			best, found = parsed, line
		}
	}
	if found == "" {
		return "", fmt.Errorf("в репозитории нет тегов вида v1.2.3")
	}
	return found, nil
}

// updateAvailable отвечает по кэшу — строка статуса в сеть не ходит.
func updateAvailable() (string, bool) {
	root, err := repoRoot()
	if err != nil {
		return "", false
	}
	cache, ok := readCache(root)
	if !ok || !newer(version, cache.Latest) {
		return "", false
	}
	return cache.Latest, true
}

// autoCheck раз в сутки запускает проверку отдельным процессом. Ждать его
// нельзя: строка статуса рисуется на каждый чих и должна возвращаться сразу.
func autoCheck() {
	if os.Getenv("CLAUDESTATUS_NO_AUTO_UPDATE") != "" {
		return
	}
	root, err := repoRoot()
	if err != nil {
		return
	}
	if cache, ok := readCache(root); ok && time.Since(time.Unix(cache.CheckedAt, 0)) < updateCheckInterval {
		return
	}
	exe, err := selfPath()
	if err != nil {
		return
	}

	// Отметку времени ставим до запуска: иначе несколько сессий разом
	// поднимут по своей проверке.
	touchCache(root)

	cmd := exec.Command(exe, "check", "--quiet")
	cmd.Dir = root
	// Вывод отвязываем от нашего: Claude Code читает stdout строки статуса
	// и ждал бы закрытия трубы фоновым процессом.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if cmd.Start() == nil {
		_ = cmd.Process.Release()
	}
}

func cachePath(root string) string {
	return filepath.Join(root, "bin", "update-check.json")
}

func readCache(root string) (updateCache, bool) {
	data, err := os.ReadFile(cachePath(root))
	if err != nil {
		return updateCache{}, false
	}
	var cache updateCache
	if json.Unmarshal(data, &cache) != nil {
		return updateCache{}, false
	}
	return cache, true
}

func writeCache(root string, cache updateCache) error {
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	path := cachePath(root)
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

// touchCache отмечает попытку проверки, не трогая известный тег: неудачный
// fetch не должен ни гасить значок обновления, ни звать проверку каждую секунду.
func touchCache(root string) {
	cache, _ := readCache(root)
	cache.CheckedAt = time.Now().Unix()
	_ = writeCache(root, cache)
}

// repoRoot — клон, из которого запущен бинарь: bin/claudestatus лежит на один
// уровень ниже корня. Скопированный куда-то бинарь обновлять нечем.
func repoRoot() (string, error) {
	exe, err := selfPath()
	if err != nil {
		return "", err
	}
	root := filepath.Dir(filepath.Dir(exe))
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return "", fmt.Errorf("не нашёлся клон репозитория рядом с %s — обновляться можно только из клона", exe)
	}
	return root, nil
}

func gitOutput(root string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
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

// parseVersion читает и голый тег, и вывод git describe: суффикс вида
// -3-gabc1234 или -dirty означает сборку поверх тега, номер версии у неё тот же.
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
