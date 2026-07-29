mod ring;
mod snapshot;
mod statusline;

use std::path::PathBuf;
use std::time::Duration;
use tauri::image::Image;
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{Emitter, Manager, Runtime};
use tauri_plugin_autostart::ManagerExt;

/// Как часто перечитываем снапшот. Writer пишет его не чаще, чем идут
/// запросы в Claude Code, поэтому чаще смысла нет.
const REFRESH: Duration = Duration::from_secs(15);

const TRAY_ID: &str = "usage";
const POPUP_LABEL: &str = "popup";

/// Путь к writer'у рядом с приложением.
fn writer_path<R: Runtime>(app: &tauri::AppHandle<R>) -> Option<PathBuf> {
    let name = if cfg!(windows) {
        "claude-statusline.exe"
    } else {
        "claude-statusline"
    };

    // Вне бандла Tauri резолвит ресурсы в каталог исполняемого файла —
    // туда же run.sh кладёт writer.
    let path = app.path().resource_dir().ok()?.join("resources").join(name);
    path.exists().then_some(path)
}

#[tauri::command]
fn snapshot() -> snapshot::Snapshot {
    snapshot::read()
}

#[tauri::command]
fn statusline_status(app: tauri::AppHandle) -> statusline::Status {
    statusline::status(writer_path(&app).as_deref())
}

#[tauri::command]
fn install_statusline(app: tauri::AppHandle) -> Result<(), String> {
    let binary = writer_path(&app).ok_or("Writer не собран рядом с приложением")?;
    statusline::install(&binary)
}

#[tauri::command]
fn autostart_enabled(app: tauri::AppHandle) -> bool {
    app.autolaunch().is_enabled().unwrap_or(false)
}

#[tauri::command]
fn set_autostart(app: tauri::AppHandle, enabled: bool) -> Result<(), String> {
    let manager = app.autolaunch();
    let result = if enabled {
        manager.enable()
    } else {
        manager.disable()
    };
    result.map_err(|error| format!("Не удалось изменить автозапуск: {error}"))
}

#[tauri::command]
fn hide_popup(app: tauri::AppHandle) {
    if let Some(window) = app.get_webview_window(POPUP_LABEL) {
        let _ = window.hide();
    }
}

#[tauri::command]
fn quit(app: tauri::AppHandle) {
    app.exit(0);
}

/// Перерисовывает иконку трея под текущее состояние 5-часового окна.
fn refresh_tray<R: Runtime>(app: &tauri::AppHandle<R>) {
    let data = snapshot::read();
    let five = data.window("five_hour");

    let progress = five.map(|w| (w.used_percentage / 100.0) as f32);
    let level = five.map_or(snapshot::Level::Unknown, |w| w.level);

    if let Some(tray) = app.tray_by_id(TRAY_ID) {
        let pixels = ring::menu_bar_icon(progress, level);
        let icon = Image::new_owned(pixels, ring::ICON_SIZE, ring::ICON_SIZE);
        let _ = tray.set_icon(Some(icon));
        let _ = tray.set_icon_as_template(false);
    }

    let _ = app.emit("snapshot", data);
}

/// Зазор между строкой меню и попапом, как у системных меню.
const POPUP_GAP: f64 = 6.0;
/// Минимальный отступ от края экрана — иначе окно упирается в угол.
const SCREEN_MARGIN: f64 = 8.0;

/// Ставит попап по центру под иконкой трея, не задевая строку меню
/// и не вылезая за край экрана.
///
/// Координаты иконки берём прямо из события клика: плагины позиционирования
/// полагаются на запомненную позицию трея и без неё роняют окно в центр экрана.
fn place_under_tray<R: Runtime>(window: &tauri::WebviewWindow<R>, icon: tauri::Rect) {
    let (Ok(Some(monitor)), Ok(size)) = (window.current_monitor(), window.outer_size()) else {
        return;
    };

    let scale = monitor.scale_factor();
    let work_area = monitor.work_area();

    // Rect приходит в логических или физических единицах — приводим к пикселям.
    let icon_position = icon.position.to_physical::<i32>(scale);
    let icon_size = icon.size.to_physical::<u32>(scale);

    let icon_center = icon_position.x + icon_size.width as i32 / 2;
    let x = icon_center - size.width as i32 / 2;

    // Обычно верх рабочей области — нижняя граница строки меню, считать её высоту
    // вручную не нужно. Но при автоскрытии меню рабочая область занимает весь экран,
    // а строка меню всё равно раскрыта в момент клика по трею — тогда ориентируемся
    // на нижний край самой иконки, иначе попап заезжает под меню.
    let icon_bottom = icon_position.y + icon_size.height as i32;
    let top = work_area.position.y.max(icon_bottom);
    let y = top + (POPUP_GAP * scale) as i32;

    let margin = (SCREEN_MARGIN * scale) as i32;
    let left_edge = work_area.position.x + margin;
    let right_edge = work_area.position.x + work_area.size.width as i32 - size.width as i32 - margin;

    let _ = window.set_position(tauri::PhysicalPosition::new(
        x.clamp(left_edge, right_edge.max(left_edge)),
        y,
    ));
}

fn toggle_popup<R: Runtime>(app: &tauri::AppHandle<R>, icon: tauri::Rect) {
    let Some(window) = app.get_webview_window(POPUP_LABEL) else {
        return;
    };

    if window.is_visible().unwrap_or(false) {
        let _ = window.hide();
        return;
    }

    place_under_tray(&window, icon);
    let _ = window.show();
    let _ = window.set_focus();

    // Открытие попапа — сам по себе запрос свежих данных: ждать
    // очередного тика таймера пользователь не должен.
    refresh_tray(app);
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_autostart::init(
            tauri_plugin_autostart::MacosLauncher::LaunchAgent,
            None,
        ))
        .invoke_handler(tauri::generate_handler![
            snapshot,
            statusline_status,
            install_statusline,
            autostart_enabled,
            set_autostart,
            hide_popup,
            quit,
        ])
        .setup(|app| {
            // Приложение живёт в строке меню: ни иконки в Dock, ни окна в Cmd+Tab.
            #[cfg(target_os = "macos")]
            app.set_activation_policy(tauri::ActivationPolicy::Accessory);

            TrayIconBuilder::with_id(TRAY_ID)
                .icon(Image::new_owned(
                    ring::menu_bar_icon(None, snapshot::Level::Unknown),
                    ring::ICON_SIZE,
                    ring::ICON_SIZE,
                ))
                .icon_as_template(false)
                .on_tray_icon_event(|tray, event| {
                    if let TrayIconEvent::Click {
                        button: MouseButton::Left,
                        button_state: MouseButtonState::Up,
                        rect,
                        ..
                    } = event
                    {
                        toggle_popup(tray.app_handle(), rect);
                    }
                })
                .build(app)?;

            let handle = app.handle().clone();
            refresh_tray(&handle);

            std::thread::spawn(move || loop {
                std::thread::sleep(REFRESH);
                refresh_tray(&handle);
            });

            Ok(())
        })
        .on_window_event(|window, event| {
            // Клик мимо попапа закрывает его — как ведут себя системные меню.
            if let tauri::WindowEvent::Focused(false) = event {
                if window.label() == POPUP_LABEL {
                    let _ = window.hide();
                }
            }
        })
        .build(tauri::generate_context!())
        .expect("не удалось собрать приложение")
        .run(|_app, event| {
            // Закрытие попапа не должно завершать приложение.
            if let tauri::RunEvent::ExitRequested { api, code, .. } = event {
                if code.is_none() {
                    api.prevent_exit();
                }
            }
        });
}
