// Пакет divoom — подкоманда `claudestatus divoom`: панель с лимитами Claude
// на экране Divoom Times Gate.
//
// Читает тот же снимок лимитов, что пишет строка статуса, рисует
// панель 128×128 и отдаёт её устройству. Кадр устройство скачивает само:
// мост поднимает локальный HTTP-сервер и присылает ссылку командой
// Device/PlayGif — заливка пикселей (Draw/SendHttpGif) на прошивке Times Gate
// принимается, но ничего не рисует.
package divoom

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	// Снапшот переписывается на каждый вызов строки статуса, то есть часто:
	// опрашиваем бодро, чтение файла ничего не стоит.
	pollInterval = 5 * time.Second
	// Устройство моргает загрузкой на каждый кадр, поэтому сдвиг обратного
	// отсчёта столько ждёт. Изменившиеся проценты этот порог не соблюдают:
	// ради них панель и висит на стене.
	minSendInterval = 30 * time.Second
	// Периодически повторяем последний кадр: устройство могло перезагрузиться
	// или потерять картинку, а снапшот при этом не меняется неделями.
	resendAfter = 15 * time.Minute
	// Сколько ждём, пока устройство придёт за кадром.
	fetchTimeout = 20 * time.Second
)

const usage = `claudestatus divoom — панель лимитов на экране Divoom Times Gate.

Использование:
  claudestatus divoom              держать панель обновлённой (работает, пока запущен)
  claudestatus divoom login        привязать устройство через аккаунт Divoom
  claudestatus divoom once         отправить панель один раз и выйти
  claudestatus divoom screen [N]   показать или занять экран 1–5
  claudestatus divoom preview FILE сохранить кадр в файл, не трогая устройство

Настройки — divoom.json в каталоге приложения, создаёт login.
`

// Run — точка входа подкоманды. Ошибки возвращаются наверх: печатает их и
// выбирает код возврата CLI, а не пакет.
func Run(args []string) error {
	if len(args) == 0 {
		return run(false)
	}

	switch args[0] {
	case "login":
		return login()
	case "once":
		return run(true)
	case "screen":
		return screen(args[1:])
	case "preview":
		if len(args) < 2 {
			return fmt.Errorf("укажите файл: claudestatus divoom preview panel.gif")
		}
		data, _, err := render(readSnapshot())
		if err != nil {
			return err
		}
		return os.WriteFile(args[1], data, 0o644)
	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("неизвестная команда: %s\n\n%s", args[0], usage)
	}
}

func run(once bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if cfg.IP == "" {
		ip, name, deviceID, err := discover()
		if err != nil {
			return err
		}
		fmt.Printf("Найдено устройство %s: %s\n", name, ip)
		cfg.IP, cfg.DeviceID = ip, deviceID
		if err := cfg.save(); err != nil {
			return err
		}
	}

	// Идентификатор устройства мог не попасть в конфиг: при заданном вручную IP
	// автопоиск не запускался, а раскладку экранов без него не спросить.
	if cfg.DeviceID == 0 {
		if _, _, deviceID, err := discover(); err == nil {
			cfg.DeviceID = deviceID
			_ = cfg.save()
		}
	}

	// Запоминаем чужой циферблат до того, как займём экран: вернуть его при
	// удалении иначе будет неоткуда.
	if cfg.PrevClockID == 0 && cfg.DeviceID != 0 {
		if clocks, independence, err := layout(cfg.DeviceID); err == nil && cfg.LcdIndex < len(clocks) {
			cfg.PrevClockID, cfg.PrevIndependence = clocks[cfg.LcdIndex], independence
			_ = cfg.save()
		}
	}

	host, err := localIP(cfg.IP)
	if err != nil {
		return fmt.Errorf("не удалось определить свой адрес в сети устройства: %w", err)
	}

	server := newAssets(cfg.Port, cfg.IP)
	if err := server.listen(); err != nil {
		return err
	}
	if server.port != cfg.Port {
		cfg.Port = server.port
		_ = cfg.save()
	}

	target := device{ip: cfg.IP, token: cfg.LocalToken, lcd: cfg.LcdIndex}

	var lastHash, lastUsage string
	var lastSent time.Time

	send := func(wait bool) error {
		state := readSnapshot()
		usage := state.usageKey()

		data, hash, err := render(state)
		if err != nil {
			return err
		}

		switch {
		case hash == lastHash:
			// Кадр тот же — повторяем изредка, чтобы устройство не потеряло
			// картинку после своей перезагрузки.
			if time.Since(lastSent) < resendAfter {
				return nil
			}
		case usage == lastUsage && !lastSent.IsZero() && time.Since(lastSent) < minSendInterval:
			// Изменился только обратный отсчёт — не гоняем устройство ради него.
			return nil
		}

		url := server.publish(host, hash, data)
		if err := target.showGif(url); err != nil {
			return err
		}
		lastHash, lastUsage, lastSent = hash, usage, time.Now()

		// Команда доставляется мгновенно, а за картинкой устройство приходит
		// отдельным запросом — при разовой отправке нельзя выходить раньше,
		// иначе сервер закроется до того, как кадр заберут.
		if wait && !server.awaitFetch(url, fetchTimeout) {
			return fmt.Errorf("устройство не забрало кадр за %s", fetchTimeout)
		}
		return nil
	}

	if err := send(once); err != nil {
		return err
	}
	if once {
		return nil
	}

	// Второй мост на то же устройство слал бы кадры вперемешку с первым.
	if err := takeLock(); err != nil {
		return err
	}
	defer dropLock()

	// По сигналу возвращаем экрану прежний циферблат: иначе на нём останется
	// последний кадр, за которым уже некому приходить.
	stopping := make(chan os.Signal, 1)
	signal.Notify(stopping, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stopping
		_ = target.restoreScreen(cfg.PrevClockID, cfg.PrevIndependence)
		dropLock()
		os.Exit(0)
	}()

	fmt.Printf("Панель на экране %d устройства %s, обновление каждые %s\n",
		cfg.LcdIndex, cfg.IP, pollInterval)

	// Ошибки в цикле не роняют мост: устройство могли выключить на ночь.
	// Но и висеть вечно рядом с выключенным устройством незачем — после
	// giveUpAfter мост уходит, а строка статуса поднимет его, когда Times Gate
	// вернётся в сеть.
	lastSeen := time.Now()
	for range time.Tick(pollInterval) {
		if err := send(false); err != nil {
			fmt.Fprintln(os.Stderr, err)
			if time.Since(lastSeen) > giveUpAfter {
				return fmt.Errorf("устройство недоступно дольше %s, выхожу", giveUpAfter)
			}
			continue
		}
		lastSeen = time.Now()
	}
	return nil
}
