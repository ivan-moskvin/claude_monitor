package divoom

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/ivan-moskvin/claude_monitor/i18n"
)

// The cloud directory: it answers with the devices sitting behind the same
// public address. No authorization required.
const lanDirectory = "https://app.divoom-gz.com/Device/ReturnSameLANDevice"

type device struct {
	ip  string
	lcd int
}

// call sends a command to the device. HTTP 200 always comes back, so the real
// result is in error_code: it is sometimes a number and sometimes a string with
// the text of the error — "Request data illegal json" means an unknown command.
func (d device) call(payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := http.Client{Timeout: 8 * time.Second}
	response, err := client.Post("http://"+d.ip+"/post", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf(i18n.T("the device is unreachable: %w"), err)
	}
	defer response.Body.Close()

	var result struct {
		ErrorCode json.RawMessage `json:"error_code"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return fmt.Errorf(i18n.T("the device answered with something other than JSON: %w"), err)
	}

	code := string(bytes.Trim(result.ErrorCode, `"`))
	if code != "0" && code != "" {
		return fmt.Errorf(i18n.T("the device rejected the command: %s"), code)
	}
	return nil
}

// showGif asks the device to download a GIF by its link and show it on its
// screen. The device pulls the picture itself — pushing the frame as pixels
// (Draw/SendHttpGif) is accepted by this firmware but draws nothing.
func (d device) showGif(url string) error {
	lcdArray := []int{0, 0, 0, 0, 0}
	lcdArray[d.lcd] = 1

	return d.call(map[string]any{
		"Command":  "Device/PlayGif",
		"LcdArray": lcdArray,
		"FileName": []string{url},
	})
}

// layout asks the cloud what the screens of the device are showing. Needed
// once — to remember somebody else's clock face and give it back on uninstall.
func layout(deviceID int) (clockIDs []int, independence int, err error) {
	url := fmt.Sprintf("https://app.divoom-gz.com/Channel/Get5LcdInfoV2?DeviceType=LCD&DeviceId=%d", deviceID)
	client := http.Client{Timeout: 15 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()

	var result struct {
		LcdIndependence     int `json:"LcdIndependence"`
		LcdIndependenceList []struct {
			LcdList []struct {
				LcdClockID int `json:"LcdClockId"`
			} `json:"LcdList"`
		} `json:"LcdIndependenceList"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, 0, err
	}
	if len(result.LcdIndependenceList) == 0 {
		return nil, 0, errors.New(i18n.T("the device did not return its screen layout"))
	}

	for _, entry := range result.LcdIndependenceList[0].LcdList {
		clockIDs = append(clockIDs, entry.LcdClockID)
	}
	return clockIDs, result.LcdIndependence, nil
}

// restoreScreen gives the screen back the clock face it had before the bridge.
// Without that the screen keeps the last frame and goes into an endless loading
// state as soon as the bridge stops answering the requests of the device.
func (d device) restoreScreen(clockID, independence int) error {
	if clockID == 0 {
		return errors.New(i18n.T("the previous clock face is unknown"))
	}
	return d.call(map[string]any{
		"Command":         "Channel/SetClockSelectId",
		"LcdIndependence": independence,
		"LcdIndex":        d.lcd,
		"ClockId":         clockID,
	})
}

// discover looks for a Times Gate on the local network through the cloud
// directory.
func discover() (ip string, name string, deviceID int, err error) {
	client := http.Client{Timeout: 15 * time.Second}
	response, err := client.Post(lanDirectory, "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", "", 0, fmt.Errorf(i18n.T("the device directory is unreachable: %w"), err)
	}
	defer response.Body.Close()

	var result struct {
		DeviceList []struct {
			DeviceName      string `json:"DeviceName"`
			DevicePrivateIP string `json:"DevicePrivateIP"`
			DeviceID        int    `json:"DeviceId"`
		} `json:"DeviceList"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", "", 0, err
	}
	if len(result.DeviceList) == 0 {
		return "", "", 0, errors.New(i18n.T("no Divoom devices are visible on this network"))
	}

	first := result.DeviceList[0]
	return first.DevicePrivateIP, first.DeviceName, first.DeviceID, nil
}

// localIP — the address the device will see us at. The UDP connection sends
// nothing but makes the system pick the right interface: walking the interfaces
// by hand would mean guessing about VPNs and bridges.
func localIP(target string) (string, error) {
	conn, err := net.Dial("udp", target+":80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}
