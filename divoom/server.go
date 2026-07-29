package divoom

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Сколько прошлых кадров держим доступными. Устройство приходит за файлом
// с задержкой и может перечитать его после своей перезагрузки — если ссылка
// к тому моменту протухнет, на экране останется пустота.
const keepFrames = 3

// assets — HTTP-сервер, с которого устройство забирает кадры. Порт
// фиксированный: после перезапуска моста на экране висит ссылка со старого
// запуска, и она должна продолжать работать.
type assets struct {
	port int
	// Кому позволено забирать кадр. Панель показывает расход лимитов, и в
	// чужой сети её незачем отдавать кому попало: соседний ноутбук в кафе
	// такой же сосед по подсети, как и наш Times Gate.
	allowIP string
	// Пути, за которыми устройство уже приходило. Команда доставляется
	// мгновенно, а картинка едет отдельным запросом — и только он
	// доказывает, что кадр реально забрали.
	fetched chan string

	mu     sync.Mutex
	frames map[string][]byte
	order  []string
}

func newAssets(port int, allowIP string) *assets {
	return &assets{port: port, allowIP: allowIP, frames: make(map[string][]byte), fetched: make(chan string, 8)}
}

// awaitFetch ждёт, пока устройство заберёт кадр по этой ссылке.
func (a *assets) awaitFetch(url string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case path := <-a.fetched:
			if strings.HasSuffix(url, path) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

// listen занимает порт до того, как мост начнёт слать команды: раньше ошибка
// ListenAndServe глоталась, мост считал, что всё хорошо, а устройство не могло
// забрать кадр — на экране висела вечная загрузка без единого намёка на причину.
func (a *assets) listen() error {
	socket, err := net.Listen("tcp", ":"+strconv.Itoa(a.port))
	if err != nil {
		// Порт занят кем-то ещё — берём любой свободный и запоминаем его,
		// чтобы после перезапуска моста ссылка на экране осталась рабочей.
		socket, err = net.Listen("tcp", ":0")
		if err != nil {
			return fmt.Errorf("не удалось открыть порт для кадров: %w", err)
		}
		a.port = socket.Addr().(*net.TCPAddr).Port
		fmt.Printf("Порт занят, кадры отдаём на %d\n", a.port)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.serve)

	// Таймауты, чтобы подвисшее соединение не держало мост: обслуживаем мы
	// одного клиента с одним маленьким файлом.
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	go func() {
		_ = server.Serve(socket)
	}()
	return nil
}

func (a *assets) serve(w http.ResponseWriter, r *http.Request) {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err != nil || (a.allowIP != "" && host != a.allowIP) {
		http.NotFound(w, r)
		return
	}

	a.mu.Lock()
	data, ok := a.frames[r.URL.Path]
	a.mu.Unlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)

	select {
	case a.fetched <- r.URL.Path:
	default:
	}
}

// publish кладёт кадр и отдаёт ссылку на него. Хэш в имени — то, что
// заставляет устройство перекачать картинку: без смены адреса оно показывает
// прежнюю.
func (a *assets) publish(hostIP, hash string, data []byte) string {
	path := "/panel-" + hash + ".gif"

	a.mu.Lock()
	if _, exists := a.frames[path]; !exists {
		a.frames[path] = data
		a.order = append(a.order, path)
		for len(a.order) > keepFrames {
			delete(a.frames, a.order[0])
			a.order = a.order[1:]
		}
	}
	a.mu.Unlock()

	return fmt.Sprintf("http://%s:%d%s", hostIP, a.port, path)
}
