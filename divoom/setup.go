package divoom

import (
	"fmt"
	"os"
)

// on находит Times Gate в сети и включает панель.
func on() error {
	ip, name, deviceID, err := discover()
	if err != nil {
		return err
	}

	cfg, _ := loadConfig()
	cfg.IP, cfg.DeviceID = ip, deviceID
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	if err := cfg.save(); err != nil {
		return err
	}

	fmt.Printf("Найдено устройство %s: %s\n", name, ip)

	EnsureRunning()
	if running() {
		fmt.Printf("Панель включена на экране %d\n", cfg.LcdIndex+1)
	}
	return nil
}

// off убирает панель: возвращает экрану его циферблат и забывает устройство.
func off() error {
	if running() {
		Stop()
	} else {
		restore()
	}

	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Println("Панель выключена, экрану возвращён его циферблат")
	return nil
}
