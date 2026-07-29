package divoom

import (
	"fmt"
	"strconv"
)

// У Times Gate пять экранов. Внутри они нумеруются с нуля, но человеку
// показываем 1–5: именно так они подписаны в приложении Divoom.
const screenCount = 5

// screen показывает или меняет экран, занятый панелью. Смена — не просто
// правка конфига: прежнему экрану надо вернуть его циферблат, иначе на нём
// останется мёртвая картинка, а мост должен переехать на новый.
func screen(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		fmt.Printf("Панель на экране %d из %d\n", cfg.LcdIndex+1, screenCount)
		return nil
	}

	number, err := strconv.Atoi(args[0])
	if err != nil || number < 1 || number > screenCount {
		return fmt.Errorf("экран — число от 1 до %d, а не %q", screenCount, args[0])
	}

	if number-1 == cfg.LcdIndex {
		fmt.Printf("Панель и так на экране %d\n", number)
		return nil
	}

	// Мост держит экран, поэтому сначала отпускаем прежний — Stop дожидается,
	// пока мост вернёт туда циферблат.
	wasRunning := running()
	if wasRunning {
		Stop()
	}

	cfg.LcdIndex = number - 1
	// Что было на новом экране, мы ещё не знаем — мост запомнит при запуске.
	cfg.PrevClockID, cfg.PrevIndependence = 0, 0
	if err := cfg.save(); err != nil {
		return err
	}

	fmt.Printf("Панель переехала на экран %d\n", number)
	if wasRunning {
		EnsureRunning()
	}
	return nil
}
