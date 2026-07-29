package divoom

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Устройство обновляет панель, только пока мост запущен. Запускать его руками
// никто не станет, поэтому строка статуса поднимает мост сама — но лишь когда
// Times Gate действительно есть в сети. Нет устройства — нет и процесса.
const (
	lockName = "divoom.pid"
	// Проверка при каждом вызове строки статуса: должна быть незаметной.
	probeTimeout = 400 * time.Millisecond
	// Устройство выключили — мост не висит вечно: выходит, а строка статуса
	// поднимет его заново, когда Times Gate вернётся.
	giveUpAfter = 10 * time.Minute
)

func lockPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", lockName), nil
}

// EnsureRunning поднимает мост в фоне, если устройство доступно, а мост ещё не
// запущен. Вызывается из строки статуса и обязана молчать: любая проблема
// здесь не повод портить строку.
func EnsureRunning() {
	cfg, err := loadConfig()
	if err != nil || cfg.LocalToken == 0 {
		// Мост не настроен — это не ошибка, у большинства нет Times Gate.
		return
	}
	if running() {
		return
	}
	if cfg.IP == "" || !reachable(cfg.IP) {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "divoom")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	// Отвязываем от группы процессов Claude Code: иначе мост умрёт вместе с
	// вызовом строки статуса, ради которого его и подняли.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	_ = cmd.Start()
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
}

// running — жив ли уже поднятый мост, по pid-файлу.
func running() bool {
	path, err := lockPath()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	// Сигнал 0 ничего не делает процессу, но отвечает, существует ли он.
	return syscall.Kill(pid, 0) == nil
}

func takeLock() error {
	path, err := lockPath()
	if err != nil {
		return err
	}
	if running() {
		return fmt.Errorf("мост уже запущен")
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
}

func dropLock() {
	if path, err := lockPath(); err == nil {
		_ = os.Remove(path)
	}
}

// reachable — отвечает ли устройство на своём порту. Полноценную команду не
// шлём: строка статуса не должна ждать ответа прошивки.
func reachable(ip string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "80"), probeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
