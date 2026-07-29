package divoom

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ivan-moskvin/claude_monitor/paths"
)

const configName = "divoom.json"

// Порт локального сервера с кадрами по умолчанию.
const defaultPort = 8477

type config struct {
	// IP устройства. Пустой — ищем в сети заново.
	IP string `json:"ip"`
	// Экран 0–4, на который отдаём панель. Остальные не трогаем.
	LcdIndex int `json:"lcd_index"`
	// Порт локального сервера с кадрами; занят — берётся свободный.
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
	return paths.File(configName)
}

func loadConfig() (config, error) {
	cfg := config{LcdIndex: 4, Port: defaultPort}

	path, err := configPath()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, fmt.Errorf("панель не включена — claudestatus divoom on")
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("%s не разбирается: %w", path, err)
	}
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	return cfg, nil
}

func (c config) save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
