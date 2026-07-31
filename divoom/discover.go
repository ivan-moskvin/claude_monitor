package divoom

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/ivan-moskvin/claude_monitor/i18n"
)

// The cloud directory: it answers with the devices sitting behind the same
// public address. No authorization required. A variable, so that reading its
// answer can be checked against a local server instead of the cloud.
var lanDirectory = "https://app.divoom-gz.com/Device/ReturnSameLANDevice"

const (
	// The whole network is knocked on at once: a device answers in
	// milliseconds, and a search the human waits for must not take longer than
	// a couple of seconds.
	scanWorkers = 128
	scanTimeout = 1200 * time.Millisecond

	// How many addresses of one network are knocked on. A /22 is the widest
	// that still fits the couple of seconds above; a /16 — let alone the /8 a
	// 10.x.x.x network can be — is millions of addresses and no search at all,
	// so a wider network is narrowed to the /24 around our own address.
	maxScanHosts   = 1024
	narrowedPrefix = 24
)

// found — a Divoom device reachable right now. The address is what we talk to;
// the name and the id come from the directory and stay empty for a device the
// scan found on its own.
type found struct {
	ip   string
	name string
	mac  string
	id   int
}

func (f found) label() string {
	if f.name == "" {
		return i18n.T("Divoom device")
	}
	return f.name
}

// devicesOnNetwork — every Divoom we can reach. The scan of the network says
// who is really there, the directory only adds the names: its records are kept
// by public address and go stale, while the address of a device is issued by
// DHCP and changes on its own.
func devicesOnNetwork() ([]found, error) {
	// The sweep and the directory take seconds each and know nothing about one
	// another — the human waits for both at once.
	var (
		cloud    []found
		cloudErr error
		wg       sync.WaitGroup
	)
	wg.Go(func() {
		cloud, cloudErr = askDirectory()
	})

	list := scanLAN()
	wg.Wait()

	known := make(map[string]int, len(list))
	for i, entry := range list {
		known[entry.ip] = i
	}

	for _, entry := range cloud {
		if i, ok := known[entry.ip]; ok {
			list[i].name, list[i].mac, list[i].id = entry.name, entry.mac, entry.id
			continue
		}
		// A device the scan missed — the network may keep its clients apart —
		// still counts if it answers us directly.
		if speaks(entry.ip) {
			list = append(list, entry)
		}
	}

	if len(list) == 0 {
		if cloudErr != nil {
			return nil, cloudErr
		}
		return nil, errors.New(i18n.T("no Divoom devices are visible on this network"))
	}

	sort.Slice(list, func(i, j int) bool {
		return bytes.Compare(net.ParseIP(list[i].ip).To16(), net.ParseIP(list[j].ip).To16()) < 0
	})
	return list, nil
}

// match finds the device the config points at. The id and the MAC outlive a
// change of address, so they are asked first; a config from an older version
// has neither and only knows the address.
func match(list []found, cfg config) (found, bool) {
	for _, entry := range list {
		if cfg.DeviceID != 0 && entry.id == cfg.DeviceID {
			return entry, true
		}
	}
	for _, entry := range list {
		if cfg.MAC != "" && entry.mac == cfg.MAC {
			return entry, true
		}
	}
	// The address alone is the weakest sign — DHCP hands it to somebody else
	// eventually — but with a single device around there is nothing to confuse
	// it with.
	if cfg.DeviceID == 0 && cfg.MAC == "" {
		for _, entry := range list {
			if entry.ip == cfg.IP {
				return entry, true
			}
		}
	}
	return found{}, false
}

// locate finds the chosen device again and writes its current address into the
// config. The known address is tried first — a sweep of the network on every
// start of the bridge would be paid for on every run, while the address usually
// holds. Nobody is at the keyboard here, so a device that is gone is replaced
// only when the network has exactly one Divoom to offer.
func locate(cfg *config) error {
	if cfg.IP != "" && speaks(cfg.IP) {
		return nil
	}

	list, err := devicesOnNetwork()
	if err != nil {
		return err
	}
	picked, ok := match(list, *cfg)
	if !ok {
		if len(list) != 1 {
			return errors.New(i18n.T("the chosen device is not on the network — choose again: claudestatus divoom on"))
		}
		picked = list[0]
	}

	cfg.IP = picked.ip
	// The scan alone knows neither the id nor the MAC: what the directory once
	// told us must not be wiped by a search that went without it.
	if picked.id != 0 {
		cfg.DeviceID = picked.id
	}
	if picked.mac != "" {
		cfg.MAC = picked.mac
	}
	if picked.name != "" {
		cfg.Name = picked.name
	}

	// Only the address is ours to write: the screen and the on/off flag may have
	// been changed by the human while we were looking.
	return update(func(saved *config) {
		saved.IP, saved.DeviceID, saved.MAC, saved.Name = cfg.IP, cfg.DeviceID, cfg.MAC, cfg.Name
	})
}

func askDirectory() ([]found, error) {
	client := http.Client{Timeout: 15 * time.Second}
	response, err := client.Post(lanDirectory, "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf(i18n.T("the device directory is unreachable: %w"), err)
	}
	defer response.Body.Close()

	var result struct {
		DeviceList []struct {
			DeviceName      string `json:"DeviceName"`
			DevicePrivateIP string `json:"DevicePrivateIP"`
			DeviceMac       string `json:"DeviceMac"`
			DeviceID        int    `json:"DeviceId"`
		} `json:"DeviceList"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}

	list := make([]found, 0, len(result.DeviceList))
	for _, entry := range result.DeviceList {
		if entry.DevicePrivateIP == "" {
			continue
		}
		list = append(list, found{
			ip:   entry.DevicePrivateIP,
			name: entry.DeviceName,
			mac:  entry.DeviceMac,
			id:   entry.DeviceID,
		})
	}
	return list, nil
}

// scanLAN knocks on every address of the networks we sit in. This is the search
// that keeps working without the directory — and the only one that survives a
// device changing its address.
func scanLAN() []found {
	var addresses []string
	for _, network := range localNetworks() {
		addresses = append(addresses, hostsOf(network)...)
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		devices []found
	)
	gate := make(chan struct{}, scanWorkers)

	for _, ip := range addresses {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			gate <- struct{}{}
			defer func() { <-gate }()

			if speaks(ip) {
				mu.Lock()
				devices = append(devices, found{ip: ip})
				mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()

	return devices
}

// speaks tells a Divoom from any other box that keeps port 80 open: it answers
// a command with error_code, while a router or a printer has nothing to say to
// /post at all.
func speaks(ip string) bool {
	client := http.Client{Timeout: scanTimeout}
	response, err := client.Post(postURL(ip), "application/json",
		bytes.NewReader([]byte(`{"Command":"Channel/GetAllConf"}`)))
	if err != nil {
		return false
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1024))
	return err == nil && bytes.Contains(body, []byte("error_code"))
}

// localNetworks — every private IPv4 network this machine sits in, taken from
// the interface with its own mask: a /16 or a /22 is as common on a router as a
// /24, and cutting the address down to three octets would leave the device
// unreachable for no reason other than where the mask happens to fall.
func localNetworks() []netip.Prefix {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var networks []netip.Prefix
	seen := make(map[netip.Prefix]bool)

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			network, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := network.IP.To4()
			if ip4 == nil || !ip4.IsPrivate() {
				continue
			}
			own, ok := netip.AddrFromSlice(ip4)
			if !ok {
				continue
			}
			bits, _ := network.Mask.Size()
			scanned := scanRange(own.Unmap(), bits)
			if !seen[scanned] {
				seen[scanned] = true
				networks = append(networks, scanned)
			}
		}
	}
	return networks
}

// scanRange — the part of the network around own that is small enough to knock
// on address by address.
func scanRange(own netip.Addr, bits int) netip.Prefix {
	if bits < 0 || bits > own.BitLen() {
		bits = narrowedPrefix
	}
	if hosts := 1 << (own.BitLen() - bits); hosts > maxScanHosts {
		bits = narrowedPrefix
	}
	return netip.PrefixFrom(own, bits).Masked()
}

// hostsOf — the addresses of a network that can belong to a device: neither the
// network address itself nor the broadcast one at the end.
func hostsOf(network netip.Prefix) []string {
	var hosts []string
	for addr := network.Addr().Next(); network.Contains(addr); addr = addr.Next() {
		hosts = append(hosts, addr.String())
	}
	if len(hosts) > 0 {
		hosts = hosts[:len(hosts)-1]
	}
	return hosts
}
