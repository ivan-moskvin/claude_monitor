// Команда claude-divoom — панель с лимитами Claude на экране Divoom Times Gate.
//
// Читает тот же ~/.claude/usage-snapshot.json, что и строка статуса, рисует
// панель 128×128 и отдаёт её устройству. Кадр устройство скачивает само:
// мост поднимает локальный HTTP-сервер и присылает ссылку командой
// Device/PlayGif — заливка пикселей (Draw/SendHttpGif) на прошивке Times Gate
// принимается, но ничего не рисует.
package main

import (
	"flag"
	"fmt"
	"os"
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

func main() {
	doLogin := flag.Bool("login", false, "получить LocalToken устройства через аккаунт Divoom")
	once := flag.Bool("once", false, "отправить панель один раз и выйти")
	preview := flag.String("preview", "", "сохранить кадр в файл вместо отправки — смотреть панель, не трогая устройство")
	flag.Parse()

	if *doLogin {
		if err := login(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if *preview != "" {
		data, _, err := render(readSnapshot())
		if err == nil {
			err = os.WriteFile(*preview, data, 0o644)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := run(*once); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(once bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if cfg.IP == "" {
		ip, name, err := discover()
		if err != nil {
			return err
		}
		fmt.Printf("Найдено устройство %s: %s\n", name, ip)
		cfg.IP = ip
		if err := cfg.save(); err != nil {
			return err
		}
	}

	host, err := localIP(cfg.IP)
	if err != nil {
		return fmt.Errorf("не удалось определить свой адрес в сети устройства: %w", err)
	}

	server := newAssets(cfg.Port)
	if err := server.listen(); err != nil {
		return err
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

	fmt.Printf("Панель на экране %d устройства %s, обновление каждые %s\n",
		cfg.LcdIndex, cfg.IP, pollInterval)

	// Ошибки в цикле не роняют мост: устройство могли выключить на ночь,
	// а мост должен сам подхватить его обратно.
	for range time.Tick(pollInterval) {
		if err := send(false); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	return nil
}
