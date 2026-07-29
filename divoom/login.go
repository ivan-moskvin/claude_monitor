package divoom

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const cloudBase = "https://appin.divoom-gz.com"

// login добывает LocalToken устройства через аккаунт Divoom и сохраняет
// настройки. Пароль нужен один раз: дальше мост работает только по локальной
// сети, а в файл попадает лишь токен устройства.
func login() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Email аккаунта Divoom: ")
	email, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	email = strings.TrimSpace(email)

	password, err := readPassword(reader)
	if err != nil {
		return err
	}

	sum := md5.Sum([]byte(password))
	var auth struct {
		ReturnCode    int             `json:"ReturnCode"`
		ReturnMessage string          `json:"ReturnMessage"`
		UserID        json.RawMessage `json:"UserId"`
		Token         json.RawMessage `json:"Token"`
	}
	if err := cloudCall("UserLogin", map[string]any{
		"Email":          email,
		"Password":       hex.EncodeToString(sum[:]),
		"CountryISOCode": "US",
		"Language":       "en",
		"TimeZone":       "UTC",
	}, &auth); err != nil {
		return err
	}
	if auth.ReturnCode != 0 {
		return fmt.Errorf("вход не удался: %s", auth.ReturnMessage)
	}

	var list struct {
		ReturnCode int `json:"ReturnCode"`
		DeviceList []struct {
			DeviceName      string `json:"DeviceName"`
			DevicePrivateIP string `json:"DevicePrivateIP"`
			DeviceID        int    `json:"DeviceId"`
			LocalToken      int    `json:"LocalToken"`
		} `json:"DeviceList"`
	}
	if err := cloudCall("Device/GetListV2", map[string]any{
		"UserId":   json.RawMessage(auth.UserID),
		"Token":    json.RawMessage(auth.Token),
		"DeviceId": 0,
	}, &list); err != nil {
		return err
	}

	for _, entry := range list.DeviceList {
		if entry.LocalToken == 0 {
			continue
		}
		cfg, _ := loadConfig()
		cfg.IP, cfg.LocalToken, cfg.DeviceID = entry.DevicePrivateIP, entry.LocalToken, entry.DeviceID
		if cfg.Port == 0 {
			cfg.Port = 8477
		}
		if err := cfg.save(); err != nil {
			return err
		}
		path, _ := configPath()
		fmt.Printf("Устройство %s (%s) записано в %s\n", entry.DeviceName, entry.DevicePrivateIP, path)

		// Мост поднимается из строки статуса, а её ближайший вызов может быть
		// нескоро: без этого после логина ничего не происходит и кажется, что
		// привязка не сработала.
		EnsureRunning()
		if running() {
			fmt.Printf("Панель включена на экране %d\n", cfg.LcdIndex)
		}
		return nil
	}
	return fmt.Errorf("в аккаунте нет устройств с LocalToken")
}

// readPassword гасит эхо через stty: тянуть ради одного ввода
// golang.org/x/term в проект без зависимостей не хочется.
func readPassword(reader *bufio.Reader) (string, error) {
	fmt.Print("Пароль: ")
	defer fmt.Println()

	if runtime.GOOS == "windows" {
		fmt.Println("(в этой консоли пароль будет виден при вводе)")
	} else if err := stty("-echo"); err == nil {
		defer func() { _ = stty("echo") }()
	} else {
		fmt.Println("(не удалось скрыть ввод — пароль будет виден)")
	}

	password, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(password), nil
}

func stty(arg string) error {
	cmd := exec.Command("stty", arg)
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func cloudCall(endpoint string, payload map[string]any, out any) error {
	payload["Command"] = endpoint
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := http.Client{Timeout: 15 * time.Second}
	response, err := client.Post(cloudBase+"/"+endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("облако Divoom недоступно: %w", err)
	}
	defer response.Body.Close()

	return json.NewDecoder(response.Body).Decode(out)
}
