package divoom

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ivan-moskvin/claude_monitor/i18n"
)

// The id and the MAC outlive a change of address, so they decide first; a
// config from an older version has neither and only knows the address.
func TestMatch(t *testing.T) {
	list := []found{
		{ip: "192.168.0.5", name: "Times Gate", mac: "aa:bb", id: 7},
		{ip: "192.168.0.6", name: "Pixoo", mac: "cc:dd", id: 9},
		{ip: "192.168.0.7"},
	}

	cases := []struct {
		name string
		cfg  config
		want string
	}{
		{"by id", config{DeviceID: 9, IP: "192.168.0.5"}, "192.168.0.6"},
		{"by MAC when the id is unknown", config{MAC: "cc:dd", IP: "192.168.0.5"}, "192.168.0.6"},
		// The id wins over both: a device that moved keeps its id and takes a
		// MAC-less record along with it.
		{"the id beats the address", config{DeviceID: 7, IP: "192.168.0.6"}, "192.168.0.5"},
		{"by address alone", config{IP: "192.168.0.7"}, "192.168.0.7"},
		// A config that knows the id or the MAC does not fall back to the
		// address: DHCP hands it to somebody else eventually.
		{"a known device that is gone", config{DeviceID: 11, IP: "192.168.0.5"}, ""},
		{"a MAC that is gone", config{MAC: "ee:ff", IP: "192.168.0.5"}, ""},
		{"an address that is gone", config{IP: "192.168.0.99"}, ""},
		{"nothing to go by", config{}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			picked, ok := match(list, c.cfg)
			if ok != (c.want != "") || picked.ip != c.want {
				t.Errorf("match() = %q, %t; want %q", picked.ip, ok, c.want)
			}
		})
	}
}

func TestMatchOnAnEmptyNetwork(t *testing.T) {
	if _, ok := match(nil, config{DeviceID: 7, IP: "192.168.0.5"}); ok {
		t.Error("a device was found on a network with nothing on it")
	}
}

// A device the scan found on its own has no name — the directory is what names
// them, and it is not always reachable.
func TestFoundLabel(t *testing.T) {
	if got := (found{ip: "192.168.0.5", name: "Times Gate"}).label(); got != "Times Gate" {
		t.Errorf("label() = %q; want the name from the directory", got)
	}
	if got := (found{ip: "192.168.0.5"}).label(); got != i18n.T("Divoom device") {
		t.Errorf("label() = %q; want a name to show the human", got)
	}
}

func TestAskDirectory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"DeviceList":[
			{"DeviceName":"Times Gate","DevicePrivateIP":"192.168.0.5","DeviceMac":"aa:bb","DeviceId":7},
			{"DeviceName":"Pixoo","DevicePrivateIP":"192.168.0.6","DeviceMac":"cc:dd","DeviceId":9},
			{"DeviceName":"A device that moved out","DevicePrivateIP":"","DeviceId":11}
		]}`))
	}))
	defer server.Close()
	withLanDirectory(t, server.URL)

	list, err := askDirectory()
	if err != nil {
		t.Fatalf("askDirectory: %v", err)
	}

	// A record with no address is nothing to talk to.
	if len(list) != 2 {
		t.Fatalf("askDirectory() returned %d devices; want 2", len(list))
	}
	if list[0] != (found{ip: "192.168.0.5", name: "Times Gate", mac: "aa:bb", id: 7}) {
		t.Errorf("the first device is %+v", list[0])
	}
}

func TestAskDirectoryWhenTheCloudIsSilent(t *testing.T) {
	t.Run("not JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("<html>"))
		}))
		defer server.Close()
		withLanDirectory(t, server.URL)

		if _, err := askDirectory(); err == nil {
			t.Error("an answer that is not JSON passed for a device list")
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		withLanDirectory(t, "http://127.0.0.1:1/")

		if _, err := askDirectory(); err == nil {
			t.Error("an unreachable directory went unreported")
		}
	})

	// An empty list is an answer, not a failure: the network may simply have no
	// devices registered in the cloud.
	t.Run("nothing registered", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"DeviceList":[]}`))
		}))
		defer server.Close()
		withLanDirectory(t, server.URL)

		list, err := askDirectory()
		if err != nil || len(list) != 0 {
			t.Errorf("askDirectory() = %v, %v; want an empty list", list, err)
		}
	})
}

// speaks tells a Divoom from any other box that keeps port 80 open.
func TestSpeaks(t *testing.T) {
	divoom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error_code":0}`))
	}))
	defer divoom.Close()

	printer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>Printer status: out of paper</html>"))
	}))
	defer printer.Close()

	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer refusing.Close()

	cases := map[string]struct {
		address string
		want    bool
	}{
		"a Divoom":                {address: hostOf(divoom.URL), want: true},
		"something else":          {address: hostOf(printer.URL)},
		"a box that says no":      {address: hostOf(refusing.URL)},
		"nothing at that address": {address: "127.0.0.1:1"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := speaks(c.address); got != c.want {
				t.Errorf("speaks(%s) = %t; want %t", c.address, got, c.want)
			}
		})
	}
}

func withLanDirectory(t *testing.T, url string) {
	t.Helper()

	saved := lanDirectory
	lanDirectory = url
	t.Cleanup(func() { lanDirectory = saved })
}

func hostOf(url string) string {
	return url[len("http://"):]
}
