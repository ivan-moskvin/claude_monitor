//! Установка строки статуса в `~/.claude/settings.json`.
//!
//! Writer собирается рядом с приложением, поэтому ставить его отдельно не нужно —
//! достаточно прописать путь к нему в настройках Claude Code.

use serde::Serialize;
use std::path::{Path, PathBuf};

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Status {
    /// В настройках прописан наш writer.
    pub installed: bool,
    /// Writer собран рядом с приложением — без него установка невозможна.
    pub available: bool,
    /// Чужая команда в `statusLine`, которую перезапишет установка.
    pub conflict: Option<String>,
}

fn claude_dir() -> PathBuf {
    let home = std::env::var_os("HOME")
        .map(PathBuf::from)
        .unwrap_or_default();
    home.join(".claude")
}

fn settings_path() -> PathBuf {
    claude_dir().join("settings.json")
}

/// Команда для `statusLine`. Кавычки — на случай пробелов в пути к репозиторию.
fn command_for(binary: &Path) -> String {
    format!("\"{}\"", binary.display())
}

fn read_settings(path: &Path) -> Result<serde_json::Value, String> {
    if !path.exists() {
        return Ok(serde_json::json!({}));
    }

    let text = std::fs::read_to_string(path)
        .map_err(|error| format!("Не удалось прочитать settings.json: {error}"))?;

    if text.trim().is_empty() {
        return Ok(serde_json::json!({}));
    }

    serde_json::from_str(&text)
        .map_err(|_| "settings.json не разбирается — поправьте его вручную".to_string())
}

pub fn status(binary: Option<&Path>) -> Status {
    let Some(binary) = binary else {
        return Status {
            installed: false,
            available: false,
            conflict: None,
        };
    };

    let wanted = command_for(binary);
    let existing = read_settings(&settings_path())
        .ok()
        .and_then(|settings| {
            settings
                .get("statusLine")?
                .get("command")?
                .as_str()
                .map(str::to_string)
        });

    match existing {
        Some(command) if command == wanted => Status {
            installed: true,
            available: true,
            conflict: None,
        },
        other => Status {
            installed: false,
            available: true,
            conflict: other,
        },
    }
}

pub fn install(binary: &Path) -> Result<(), String> {
    if !binary.exists() {
        return Err("Writer не собран рядом с приложением".to_string());
    }

    let path = settings_path();
    let mut settings = read_settings(&path)?;

    // Бэкап делаем до записи и только при существующем файле —
    // иначе затрём чужие настройки без возможности откатиться.
    if path.exists() {
        std::fs::copy(&path, path.with_extension("json.bak"))
            .map_err(|error| format!("Не удалось сделать бэкап settings.json: {error}"))?;
    }

    if !settings.is_object() {
        return Err("settings.json содержит не объект — поправьте его вручную".to_string());
    }

    settings["statusLine"] = serde_json::json!({
        "type": "command",
        "command": command_for(binary),
    });

    std::fs::create_dir_all(claude_dir())
        .map_err(|error| format!("Не удалось создать ~/.claude: {error}"))?;

    let mut text = serde_json::to_string_pretty(&settings)
        .map_err(|error| format!("Не удалось собрать settings.json: {error}"))?;
    text.push('\n');

    std::fs::write(&path, text)
        .map_err(|error| format!("Не удалось записать settings.json: {error}"))
}
