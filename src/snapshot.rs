//! The usage snapshot: the only contract with whoever shows the numbers outside
//! the status line — the panel on a Divoom Times Gate.
//!
//! The `rate_limits` block is copied as it came: the writer picks no fields, so
//! new windows appear on their own.

use std::time::{SystemTime, UNIX_EPOCH};

use serde_json::{Map, Value};

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

/// Days since the epoch to a calendar date, by Howard Hinnant's civil_from_days.
fn civil_from_days(days: i64) -> (i64, u32, u32) {
    let days = days + 719_468;
    let era = days.div_euclid(146_097);
    let day_of_era = days.rem_euclid(146_097);
    let year_of_era =
        (day_of_era - day_of_era / 1460 + day_of_era / 36_524 - day_of_era / 146_096) / 365;
    let year = year_of_era + era * 400;
    let day_of_year = day_of_era - (365 * year_of_era + year_of_era / 4 - year_of_era / 100);
    let shifted_month = (5 * day_of_year + 2) / 153;
    let day = (day_of_year - (153 * shifted_month + 2) / 5 + 1) as u32;
    let month = if shifted_month < 10 {
        shifted_month + 3
    } else {
        shifted_month - 9
    } as u32;
    (if month <= 2 { year + 1 } else { year }, month, day)
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

    #[test]
    fn turns_days_into_a_calendar_date() {
        assert_eq!(civil_from_days(0), (1970, 1, 1));
        assert_eq!(civil_from_days(19_723), (2024, 1, 1));
        // A leap day, which is where a home-made calendar usually goes wrong.
        assert_eq!(civil_from_days(19_782), (2024, 2, 29));
        assert_eq!(civil_from_days(20_608), (2026, 6, 4));
    }
}
