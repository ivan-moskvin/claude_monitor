package divoom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Облачный каталог: по публичному адресу отдаёт устройства из той же сети.
// Единственный запрос наружу и только при поиске IP — дальше всё локально.
const lanDirectory = "https://app.divoom-gz.com/Device/ReturnSameLANDevice"

type device struct {
	ip    string
	token int
	lcd   int
}

// call шлёт команду устройству. HTTP 200 приходит всегда, поэтому настоящий
// результат смотрим в error_code: он бывает и числом, и строкой с текстом
// ошибки — «Request data illegal json» означает неизвестную команду.
func (d device) call(payload map[string]any) error {
	payload["LocalToken"] = d.token
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := http.Client{Timeout: 8 * time.Second}
	response, err := client.Post("http://"+d.ip+"/post", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("устройство недоступно: %w", err)
	}
	defer response.Body.Close()

	var result struct {
		ErrorCode json.RawMessage `json:"error_code"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return fmt.Errorf("устройство ответило не JSON: %w", err)
	}

	code := string(bytes.Trim(result.ErrorCode, `"`))
	if code != "0" && code != "" {
		return fmt.Errorf("устройство отклонило команду: %s", code)
	}
	return nil
}

// showGif просит устройство скачать GIF по ссылке и показать его на своём
// экране. Картинку тянет само устройство — заливка кадра пикселями
// (Draw/SendHttpGif) на этой прошивке принимается, но ничего не рисует.
func (d device) showGif(url string) error {
	lcdArray := []int{0, 0, 0, 0, 0}
	lcdArray[d.lcd] = 1

	return d.call(map[string]any{
		"Command":  "Device/PlayGif",
		"LcdArray": lcdArray,
		"FileName": []string{url},
	})
}

// discover ищет Times Gate в локальной сети через облачный каталог.
func discover() (ip string, name string, err error) {
	client := http.Client{Timeout: 15 * time.Second}
	response, err := client.Post(lanDirectory, "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", "", fmt.Errorf("каталог устройств недоступен: %w", err)
	}
	defer response.Body.Close()

	var result struct {
		DeviceList []struct {
			DeviceName      string `json:"DeviceName"`
			DevicePrivateIP string `json:"DevicePrivateIP"`
		} `json:"DeviceList"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", "", err
	}
	if len(result.DeviceList) == 0 {
		return "", "", fmt.Errorf("устройств Divoom в этой сети не видно")
	}

	first := result.DeviceList[0]
	return first.DevicePrivateIP, first.DeviceName, nil
}

// localIP — адрес, с которого нас увидит устройство. Соединение UDP ничего
// не отправляет, но заставляет систему выбрать нужный интерфейс: перебирать
// интерфейсы руками пришлось бы с догадками про VPN и мосты.
func localIP(target string) (string, error) {
	conn, err := net.Dial("udp", target+":80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}
