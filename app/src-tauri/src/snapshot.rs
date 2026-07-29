//! Чтение `~/.claude/usage-snapshot.json` — единственного контракта с writer'ом.
//!
//! Здесь же живут пороги цвета и признак сброшенного окна: фронтенд получает
//! готовый `level` и не дублирует эту логику у себя.

use serde::Serialize;
use std::path::PathBuf;

/// Окна, которые умеет показывать виджет. Добавление нового начинается здесь.
const KNOWN_WINDOWS: &[(&str, &str)] = &[
    ("five_hour", "Сессия · 5 часов"),
    ("seven_day", "Неделя · 7 дней"),
    ("seven_day_opus", "Неделя · Opus"),
];

/// Снапшот старше этого возраста подписывается как устаревший.
const STALE_AFTER_SECONDS: f64 = 90.0;

#[derive(Serialize, Clone, Copy, PartialEq)]
#[serde(rename_all = "lowercase")]
pub enum Level {
    Ok,
    Warn,
    Critical,
    Expired,
}

#[derive(Serialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct Window {
    pub id: String,
    pub title: String,
    pub used_percentage: f64,
    /// Секунд до сброса окна; `None`, если сброса нет или он уже наступил.
    pub seconds_left: Option<i64>,
    pub expired: bool,
    pub level: Level,
}

#[derive(Serialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct Snapshot {
    pub windows: Vec<Window>,
    /// Возраст снапшота в секундах; `None`, если writer не записал `updated_at`.
    pub age_seconds: Option<f64>,
    pub stale: bool,
    /// Текст ошибки вместо данных — писать нечего, показываем подсказку.
    pub error: Option<String>,
}

impl Snapshot {
    pub fn failure(message: &str) -> Self {
        Self {
            windows: Vec::new(),
            age_seconds: None,
            stale: false,
            error: Some(message.to_string()),
        }
    }

    pub fn window(&self, id: &str) -> Option<&Window> {
        self.windows.iter().find(|w| w.id == id)
    }
}

pub fn snapshot_path() -> PathBuf {
    let home = std::env::var_os("HOME")
        .map(PathBuf::from)
        .unwrap_or_default();
    home.join(".claude").join("usage-snapshot.json")
}

pub fn read() -> Snapshot {
    let path = snapshot_path();

    let Ok(text) = std::fs::read_to_string(&path) else {
        return Snapshot::failure("Запустите сессию Claude Code для получения данных");
    };

    let Ok(root) = serde_json::from_str::<serde_json::Value>(&text) else {
        return Snapshot::failure("Файл со снапшотом повреждён");
    };

    let now = unix_now();
    let limits = root.get("rate_limits");

    let windows: Vec<Window> = KNOWN_WINDOWS
        .iter()
        .filter_map(|(key, title)| {
            let entry = limits?.get(key)?;
            let used = number(entry.get("used_percentage"))?;
            let resets_at = number(entry.get("resets_at")).map(normalize_epoch);

            let expired = resets_at.is_some_and(|at| at <= now);
            let seconds_left = match resets_at {
                Some(at) if !expired => Some((at - now) as i64),
                _ => None,
            };

            Some(Window {
                id: (*key).to_string(),
                title: (*title).to_string(),
                used_percentage: used,
                seconds_left,
                expired,
                level: level_for(used, expired),
            })
        })
        .collect();

    if windows.is_empty() {
        return Snapshot::failure("Запустите сессию Claude Code для получения данных");
    }

    let age_seconds = timestamp(root.get("updated_at")).map(|at| now - at);

    Snapshot {
        stale: age_seconds.is_some_and(|age| age >= STALE_AFTER_SECONDS),
        age_seconds,
        windows,
        error: None,
    }
}

/// Пороги цвета — единственное место на весь проект.
fn level_for(used: f64, expired: bool) -> Level {
    if expired {
        Level::Expired
    } else if used < 60.0 {
        Level::Ok
    } else if used < 85.0 {
        Level::Warn
    } else {
        Level::Critical
    }
}

fn unix_now() -> f64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs_f64())
        .unwrap_or_default()
}

fn number(value: Option<&serde_json::Value>) -> Option<f64> {
    value?.as_f64()
}

/// Unix-время приходит в секундах или миллисекундах — различаем по величине.
fn normalize_epoch(value: f64) -> f64 {
    if value > 1e12 {
        value / 1000.0
    } else {
        value
    }
}

/// `resets_at` — число, `updated_at` — ISO8601; принимаем оба вида.
fn timestamp(value: Option<&serde_json::Value>) -> Option<f64> {
    let value = value?;

    if let Some(seconds) = value.as_f64() {
        return Some(normalize_epoch(seconds));
    }

    let text = value.as_str()?;
    chrono::DateTime::parse_from_rfc3339(text)
        .ok()
        .map(|dt| dt.timestamp() as f64 + f64::from(dt.timestamp_subsec_millis()) / 1000.0)
}
