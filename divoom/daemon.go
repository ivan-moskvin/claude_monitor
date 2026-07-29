package divoom

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ivan-moskvin/claude_monitor/paths"
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
	return paths.File(lockName)
}

// EnsureRunning поднимает мост в фоне, если устройство доступно, а мост ещё не
// запущен. Вызывается из строки статуса и обязана молчать: любая проблема
// здесь не повод портить строку.
func EnsureRunning() {
	cfg, err := loadConfig()
	if err != nil || cfg.IP == "" {
		// Панель не включена — это не ошибка, у большинства нет Times Gate.
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
	cmd.SysProcAttr = detachAttrs()
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
	return processAlive(pid)
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

// Stop останавливает мост и убирает за ним. Зовётся из `claudestatus uninstall`:
// без этого мост переживает удаление бинаря, а экран устройства остаётся с
// панелью и уходит в вечную загрузку, когда мост всё-таки умрёт.
func Stop() {
	path, err := lockPath()
	if err != nil {
		return
	}

	if data, err := os.ReadFile(path); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
			// Где есть сигналы, мост возвращает экран сам; где нет — и там,
			// где не успел, — возвращаем за него.
			_ = terminate(pid)
			for i := 0; i < 30 && processAlive(pid); i++ {
				time.Sleep(100 * time.Millisecond)
			}
			if processAlive(pid) {
				_ = forceKill(pid)
			}
			if !gracefulStop || processAlive(pid) {
				restore()
			}
			fmt.Println("Остановлен мост Divoom")
		}
	}
	_ = os.Remove(path)
}

// restore возвращает экрану прежний циферблат по сохранённым в конфиге
// значениям. Молчит: при удалении устройство может быть выключено, и это не
// повод ронять uninstall.
func restore() {
	cfg, err := loadConfig()
	if err != nil || cfg.IP == "" || cfg.PrevClockID == 0 {
		return
	}
	target := device{ip: cfg.IP, lcd: cfg.LcdIndex}
	_ = target.restoreScreen(cfg.PrevClockID, cfg.PrevIndependence)
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
