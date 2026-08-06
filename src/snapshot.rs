//! The usage snapshot: the only contract with whoever shows the numbers outside
//! the status line — the panel on a Divoom Times Gate.
//!
//! The `rate_limits` block is copied as it came: the writer picks no fields, so
//! new windows appear on their own.

use std::time::{SystemTime, UNIX_EPOCH};

use serde_json::{Map, Value};

use crate::civil::civil_from_days;
use crate::paths;

pub const NAME: &str = "usage-snapshot.json";

/// Writes the limits down. Claude Code sometimes calls the status line without
/// any of them — the previous numbers are more honest than empty ones.
pub fn save(limits: &Map<String, Value>) -> Result<(), String> {
    if limits.is_empty() {
        return Ok(());
    }

    let dir = paths::dir()?;
    let record = serde_json::json!({
        "rate_limits": limits,
        "updated_at": now_rfc3339(),
    });
    let mut data = serde_json::to_vec(&record).map_err(|err| err.to_string())?;
    data.push(b'\n');

    // Written to a temporary file next to it and renamed: the reader never sees
    // half a record and needs no locking.
    let staged = dir.join(format!("{NAME}.tmp"));
    std::fs::write(&staged, data).map_err(|err| err.to_string())?;
    std::fs::rename(staged, dir.join(NAME)).map_err(|err| err.to_string())
}

/// The moment, spelled the way RFC 3339 wants it. Only whole seconds: the
/// snapshot says how stale the numbers are, and nothing finer is ever asked.
fn now_rfc3339() -> String {
    let seconds = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|since| since.as_secs())
        .unwrap_or(0);
    let (days, time) = (seconds / 86_400, seconds % 86_400);
    let (year, month, day) = civil_from_days(days as i64);
    let (hour, minute, second) = (time / 3600, (time % 3600) / 60, time % 60);
    format!("{year:04}-{month:02}-{day:02}T{hour:02}:{minute:02}:{second:02}Z")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn spells_a_date_the_way_rfc3339_wants_it() {
        let stamp = now_rfc3339();
        assert_eq!(stamp.len(), 20, "{stamp}");
        assert!(stamp.ends_with('Z'), "{stamp}");
        assert_eq!(&stamp[4..5], "-");
        assert_eq!(&stamp[10..11], "T");
    }
}
