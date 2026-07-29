//go:build !windows

package divoom

import "syscall"

// Работа с чужим процессом устроена по-разному на разных системах, поэтому
// живёт в файлах с build-тегами: в пакете syscall под Windows нет ни Kill,
// ни поля Setsid, и кросс-компиляция релизных бинарей падала именно на этом.

// gracefulStop — умеет ли система попросить процесс завершиться, дав ему
// прибраться. На Unix это SIGTERM, и мост успевает вернуть экрану циферблат.
const gracefulStop = true

// detachAttrs отвязывает мост от группы процессов Claude Code: иначе он умрёт
// вместе с вызовом строки статуса, ради которого его и подняли.
func detachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// processAlive — существует ли процесс. Сигнал 0 ничего ему не делает, но
// отвечает на этот вопрос.
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func terminate(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

func forceKill(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}
