package divoom

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Настройки лежат рядом со снапшотом, в каталоге Claude Code.
const configName = "divoom.json"

type config struct {
	// IP устройства. Пустой — ищем через облачный каталог при каждом запуске.
	IP string `json:"ip"`
	// LocalToken из настроек устройства: без него прошивка молча игнорирует
	// команды рисования, отвечая при этом error_code 0.
	LocalToken int `json:"local_token"`
	// Экран 0–4, на который отдаём панель. Остальные не трогаем.
	LcdIndex int `json:"lcd_index"`
	// Порт локального сервера с кадрами; фиксированный, см. assets.
	Port int `json:"port"`
	// Идентификатор устройства в облаке Divoom — по нему узнаётся раскладка экранов.
	DeviceID int `json:"device_id,omitempty"`
	// Что было на нашем экране до нас: циферблат и набор экранов, которому он
	// принадлежит. Запоминаем при первом запуске и возвращаем при удалении —
	// иначе после ухода моста экран останется с мёртвой картинкой.
	PrevClockID      int `json:"prev_clock_id,omitempty"`
	PrevIndependence int `json:"prev_independence,omitempty"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", configName), nil
}

func loadConfig() (config, error) {
	cfg := config{LcdIndex: 4, Port: 8477}

	path, err := configPath()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, fmt.Errorf("нет %s — запустите с --login", path)
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("%s не разбирается: %w", path, err)
	}
	if cfg.LocalToken == 0 {
		return cfg, fmt.Errorf("в %s нет local_token — запустите с --login", path)
	}
	if cfg.Port == 0 {
		cfg.Port = 8477
	}
	return cfg, nil
}

func (c config) save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	// Токен — доступ к устройству в локальной сети, чужим читать незачем.
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
