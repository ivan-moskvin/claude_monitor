//! The status line itself: session JSON on stdin, one line on stdout.
//!
//! We have no process of our own — the command lives for exactly one call.
//! Everything shown arrives here and now, the only exception being the update
//! check cache. Hence the rules: empty or unparsable input prints a dash rather
//! than an error, a window missing from `rate_limits` is simply not drawn, and
//! nothing waits for the network.

use std::io::Read;
use std::time::{SystemTime, UNIX_EPOCH};

use serde_json::{Map, Value};

use crate::i18n::tf;
use crate::snapshot;

const BAR_WIDTH: usize = 10;
const FIVE_HOUR_SECONDS: f64 = 5.0 * 60.0 * 60.0;
const SEVEN_DAY_SECONDS: f64 = 7.0 * 24.0 * 60.0 * 60.0;

/// Colors of the 256-color palette. Usage bars are painted by threshold, the
/// reset bar is always cyan — it shows time, not risk.
const GREEN: u8 = 35;
const ORANGE: u8 = 214;
const RED: u8 = 167;
const CYAN: u8 = 38;
const UPDATE: u8 = 178;
const EMPTY_BG: u8 = 237;
const EMPTY_FG: u8 = 250;
/// The cell that marks how much of the weekly window has passed: lighter than
/// the empty part, darker than any of the level colors.
const MARK_BG: u8 = 245;
const DARK_TEXT: u8 = 16;
const LIGHT_TEXT: u8 = 231;

/// The reasoning effort scale: the circle phases are the very symbols Claude
/// Code itself uses. The color is deliberately purple to pink so that it is
/// never confused with the green/orange/red of the usage bars.
const EFFORT: &[(&str, &str, u8)] = &[
    ("low", "○", 245),
    ("medium", "◔", 111),
    ("high", "◑", 141),
    ("xhigh", "◕", 171),
    ("max", "●", 199),
];

/// Prints the line — the default mode, and the one settings.json points at.
pub fn run(update_mark: Option<String>) {
    let mut input = String::new();
    if std::io::stdin().read_to_string(&mut input).is_err() {
        println!("—");
        return;
    }

    let Ok(session) = serde_json::from_str::<Value>(&input) else {
        println!("—");
        return;
    };

    let limits = session
        .get("rate_limits")
        .and_then(Value::as_object)
        .cloned()
        .unwrap_or_default();

    // The limits arrive here and only during a session: there is nowhere else to
    // get them. A write error is swallowed — the status line matters more.
    let _ = snapshot::save(&limits);

    println!("{}", compose(&session, &limits, update_mark, now()));
}

/// Puts the line together. Takes the moment as an argument so that a test can
/// say when it is.
fn compose(
    session: &Value,
    limits: &Map<String, Value>,
    update_mark: Option<String>,
    now: f64,
) -> String {
    let mut parts: Vec<String> = Vec::new();

    if let Some(name) = session
        .pointer("/model/display_name")
        .and_then(Value::as_str)
        && !name.is_empty()
    {
        // The level goes next to the model: how full the circle is reads even
        // without color, and it costs one character instead of a word.
        let level = session
            .pointer("/effort/level")
            .and_then(Value::as_str)
            .unwrap_or_default();
        match EFFORT.iter().find(|(candidate, _, _)| *candidate == level) {
            Some((_, mark, color)) => parts.push(colorized(&format!("{name} {mark}"), *color)),
            None => parts.push(name.to_string()),
        }
    }

    let five = limits.get("five_hour");

    if let Some(used) = five.and_then(|window| percentage(window, "used_percentage")) {
        parts.push(format!(
            "5h {}",
            labeled_bar(used, &percent_label(used), usage_color(used))
        ));
    }

    if let Some(left) = five.and_then(|window| seconds_left(window, now)) {
        let elapsed = (FIVE_HOUR_SECONDS - left as f64) / FIVE_HOUR_SECONDS * 100.0;
        parts.push(format!(
            "{} {}",
            crate::i18n::t("reset"),
            labeled_bar(elapsed, &countdown(left), CYAN)
        ));
    }

    // The weekly window goes last: it moves slowly and gets in the way of what
    // matters right now.
    let week = limits.get("seven_day");

    if let Some(used) = week.and_then(|window| percentage(window, "used_percentage")) {
        let (label, color) = (percent_label(used), usage_color(used));
        // The mark says where the week itself has got to. Without a resets_at
        // there is nothing to compare the usage against, and the bar is plain.
        let drawn = match week.and_then(|window| seconds_left(window, now)) {
            Some(left) => {
                let elapsed = (SEVEN_DAY_SECONDS - left as f64) / SEVEN_DAY_SECONDS * 100.0;
                marked_bar(used, &label, color, elapsed)
            }
            None => labeled_bar(used, &label, color),
        };
        parts.push(format!("7d {drawn}"));
    }

    // The update mark comes at the very end: it is not about the current session
    // and must not shift the limit numbers the eye is already used to.
    if let Some(tag) = update_mark {
        parts.push(colorized(&format!("↑ {tag}"), UPDATE));
    }

    if parts.is_empty() {
        return "—".to_string();
    }
    parts.join(" · ")
}

/// Draws a bar with the label centered inside it. The background spans the full
/// width — the filled part in the level color, the rest dark grey — so the edge
/// of the bar reads without a frame.
fn labeled_bar(percentage: f64, label: &str, color: u8) -> String {
    let text = centered(label);
    let filled = cells(percentage);

    let text_color = text_color(color);
    let (inside, outside): (String, String) = (
        text[..filled].iter().collect(),
        text[filled..].iter().collect(),
    );

    format!(
        "\x1b[1;48;5;{color};38;5;{text_color}m{inside}\x1b[0;48;5;{EMPTY_BG};38;5;{EMPTY_FG}m{outside}\x1b[0m"
    )
}

/// The same bar with one cell painted grey: how much of the window itself has
/// passed. A fill that has run past the mark is spent faster than the window
/// gives it back, one that trails it leaves room to spare.
fn marked_bar(percentage: f64, label: &str, color: u8, elapsed: f64) -> String {
    let text = centered(label);
    let filled = cells(percentage);
    // The mark stands on the cell the elapsed time reaches into, so a window
    // that has only just begun still marks its first cell rather than none.
    let mark = cells(elapsed).clamp(1, BAR_WIDTH) - 1;

    let mut out = String::new();
    let mut painted = None;
    for (cell, symbol) in text.iter().enumerate() {
        let style = if cell == mark {
            (0, MARK_BG, DARK_TEXT)
        } else if cell < filled {
            (1, color, text_color(color))
        } else {
            (0, EMPTY_BG, EMPTY_FG)
        };
        if painted != Some(style) {
            let (weight, background, foreground) = style;
            out.push_str(&format!(
                "\x1b[{weight};48;5;{background};38;5;{foreground}m"
            ));
            painted = Some(style);
        }
        out.push(*symbol);
    }
    out + "\x1b[0m"
}

/// The label in the middle of a bar-wide row of spaces. A label of odd length
/// does not center exactly — the leftover space goes on the left: otherwise
/// "12%" and "39m" drift noticeably towards the left edge of the bar.
fn centered(label: &str) -> Vec<char> {
    let mut label: Vec<char> = label.chars().collect();
    label.truncate(BAR_WIDTH);

    let pad = BAR_WIDTH - label.len();
    let left = pad.div_ceil(2);
    " ".repeat(left)
        .chars()
        .chain(label)
        .chain(" ".repeat(pad - left).chars())
        .collect()
}

fn cells(percentage: f64) -> usize {
    (percentage / 100.0 * BAR_WIDTH as f64)
        .round_ties_even()
        .clamp(0.0, BAR_WIDTH as f64) as usize
}

fn text_color(color: u8) -> u8 {
    if color == RED { LIGHT_TEXT } else { DARK_TEXT }
}

fn usage_color(percentage: f64) -> u8 {
    match percentage {
        percentage if percentage >= 85.0 => RED,
        percentage if percentage >= 60.0 => ORANGE,
        _ => GREEN,
    }
}

fn colorized(text: &str, color: u8) -> String {
    format!("\x1b[1;38;5;{color}m{text}\x1b[0m")
}

/// Does not right-align the number: leading spaces would end up inside the label
/// and push it right of the bar center.
fn percent_label(percentage: f64) -> String {
    format!("{}%", percentage.round() as i64)
}

fn percentage(window: &Value, field: &str) -> Option<f64> {
    window.get(field)?.as_f64()
}

/// A window that has already reset is not drawn at all — Claude Code will
/// redraw the line by itself.
fn seconds_left(window: &Value, now: f64) -> Option<i64> {
    let resets_at = normalize_epoch(window.get("resets_at")?.as_f64()?);
    let seconds = (resets_at - now) as i64;
    (seconds > 0).then_some(seconds)
}

/// Prints the hours even when they are zero: without them the label shrinks from
/// six characters to three as it crosses an hour, and the digits jump towards
/// the center of the bar.
fn countdown(seconds: i64) -> String {
    let minutes = seconds / 60;
    tf!("{0}h {1}m", minutes / 60, format!("{:02}", minutes % 60))
}

/// Accepts Unix time in seconds or in milliseconds.
fn normalize_epoch(value: f64) -> f64 {
    if value > 1e12 { value / 1000.0 } else { value }
}

/// The current time as a fraction, otherwise the remainder rounds up and the
/// countdown shows one minute too many.
fn now() -> f64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|since| since.as_secs_f64())
        .unwrap_or(0.0)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    /// The line with the escape sequences taken out — what the eye sees.
    fn plain(line: &str) -> String {
        let mut out = String::new();
        let mut rest = line;
        while let Some(start) = rest.find('\x1b') {
            out.push_str(&rest[..start]);
            match rest[start..].find('m') {
                Some(end) => rest = &rest[start + end + 1..],
                None => return out,
            }
        }
        out.push_str(rest);
        out
    }

    fn limits_of(value: Value) -> Map<String, Value> {
        value.as_object().cloned().unwrap_or_default()
    }

    #[test]
    fn draws_nothing_but_a_dash_for_an_empty_session() {
        assert_eq!(compose(&json!({}), &Map::new(), None, 0.0), "—");
    }

    #[test]
    fn draws_the_windows_it_was_given() {
        let limits = limits_of(json!({
            "five_hour": {"used_percentage": 42.0},
            "seven_day": {"used_percentage": 8.0},
        }));
        let line = plain(&compose(&json!({}), &limits, None, 0.0));

        assert!(line.contains("5h"), "{line}");
        assert!(line.contains("42%"), "{line}");
        assert!(line.contains("7d"), "{line}");
        assert!(line.contains("8%"), "{line}");
        assert!(!line.contains("reset"), "{line}");
    }

    #[test]
    fn leaves_out_a_window_that_is_not_there() {
        let limits = limits_of(json!({"seven_day": {"used_percentage": 8.0}}));
        let line = plain(&compose(&json!({}), &limits, None, 0.0));

        assert!(!line.contains("5h"), "{line}");
        assert!(line.contains("7d"), "{line}");
    }

    #[test]
    fn counts_down_to_the_reset() {
        let limits =
            limits_of(json!({"five_hour": {"used_percentage": 10.0, "resets_at": 9000.0}}));
        // Two hours and forty-one minutes before the window resets.
        let line = plain(&compose(
            &json!({}),
            &limits,
            None,
            9000.0 - (2.0 * 3600.0 + 41.0 * 60.0),
        ));

        assert!(line.contains("2h 41m"), "{line}");
    }

    #[test]
    fn leaves_out_a_window_that_has_already_reset() {
        let limits = limits_of(json!({"five_hour": {"used_percentage": 10.0, "resets_at": 100.0}}));
        let line = plain(&compose(&json!({}), &limits, None, 200.0));

        assert!(!line.contains("0h 00m"), "{line}");
        assert!(line.contains("10%"), "{line}");
    }

    #[test]
    fn reads_a_reset_stamped_in_milliseconds() {
        // Milliseconds are told from seconds by their size, so the stamp has to
        // be one a real session would carry.
        let resets_at = 1_770_000_000.0_f64;
        let limits = limits_of(json!({"five_hour": {"resets_at": resets_at * 1000.0}}));
        let line = plain(&compose(&json!({}), &limits, None, resets_at - 3600.0));

        assert!(line.contains("1h 00m"), "{line}");
    }

    #[test]
    fn shows_the_model_and_its_effort() {
        let session = json!({"model": {"display_name": "Opus 5"}, "effort": {"level": "high"}});
        let line = plain(&compose(&session, &Map::new(), None, 0.0));

        assert_eq!(line, "Opus 5 ◑");
    }

    #[test]
    fn shows_a_model_whose_effort_is_not_a_level_we_know() {
        let session = json!({"model": {"display_name": "Opus 5"}, "effort": {"level": "turbo"}});
        assert_eq!(plain(&compose(&session, &Map::new(), None, 0.0)), "Opus 5");

        let session = json!({"model": {"display_name": "Opus 5"}});
        assert_eq!(plain(&compose(&session, &Map::new(), None, 0.0)), "Opus 5");
    }

    #[test]
    fn puts_the_update_mark_last() {
        let limits = limits_of(json!({"five_hour": {"used_percentage": 42.0}}));
        let line = plain(&compose(&json!({}), &limits, Some("v1.4.0".into()), 0.0));

        assert!(line.ends_with("↑ v1.4.0"), "{line}");
    }

    #[test]
    fn paints_a_bar_by_how_full_it_is() {
        assert_eq!(usage_color(0.0), GREEN);
        assert_eq!(usage_color(59.9), GREEN);
        assert_eq!(usage_color(60.0), ORANGE);
        assert_eq!(usage_color(84.9), ORANGE);
        assert_eq!(usage_color(85.0), RED);
        assert_eq!(usage_color(100.0), RED);
    }

    /// How many of the ten cells the fill covers.
    fn filled_cells(bar: &str) -> usize {
        let empty = format!("\x1b[0;48;5;{EMPTY_BG};38;5;{EMPTY_FG}m");
        let (fill, _) = bar
            .split_once(&empty)
            .expect("a bar always has an empty part");
        plain(fill).chars().count()
    }

    #[test]
    fn fills_the_bar_in_proportion() {
        assert_eq!(filled_cells(&labeled_bar(50.0, "50%", GREEN)), 5);
        assert_eq!(filled_cells(&labeled_bar(0.0, "", GREEN)), 0);
        assert_eq!(filled_cells(&labeled_bar(100.0, "", GREEN)), 10);
        assert_eq!(filled_cells(&labeled_bar(42.0, "42%", GREEN)), 4);
    }

    /// Which of the ten cells carries the grey mark.
    fn marked_cell(bar: &str) -> usize {
        let mark = format!("\x1b[0;48;5;{MARK_BG};38;5;{DARK_TEXT}m");
        let (before, _) = bar.split_once(&mark).expect("a marked bar has a mark");
        plain(before).chars().count()
    }

    #[test]
    fn marks_how_much_of_the_week_has_passed() {
        assert_eq!(marked_cell(&marked_bar(11.0, "11%", GREEN, 60.0)), 5);
        // The mark sits on the cell the time reaches into, never outside the bar.
        assert_eq!(marked_cell(&marked_bar(11.0, "11%", GREEN, 0.0)), 0);
        assert_eq!(marked_cell(&marked_bar(11.0, "11%", GREEN, 100.0)), 9);
        // A fill that has run past the mark still leaves it visible.
        assert_eq!(marked_cell(&marked_bar(78.0, "78%", ORANGE, 30.0)), 2);
        assert_eq!(
            plain(&marked_bar(78.0, "78%", ORANGE, 30.0))
                .chars()
                .count(),
            BAR_WIDTH
        );
    }

    #[test]
    fn marks_the_week_only_when_it_says_when_it_resets() {
        let ticking = limits_of(json!({
            "seven_day": {"used_percentage": 11.0, "resets_at": 604_800.0},
        }));
        let line = compose(&json!({}), &ticking, None, 604_800.0 / 2.0);
        assert!(line.contains(&format!("48;5;{MARK_BG}")), "{line}");

        let timeless = limits_of(json!({"seven_day": {"used_percentage": 11.0}}));
        let line = compose(&json!({}), &timeless, None, 0.0);
        assert!(!line.contains(&format!("48;5;{MARK_BG}")), "{line}");
    }

    #[test]
    fn keeps_a_bar_ten_cells_wide_whatever_the_label() {
        for label in ["", "5%", "100%", "a very long label indeed"] {
            let bar = labeled_bar(40.0, label, GREEN);
            assert_eq!(plain(&bar).chars().count(), BAR_WIDTH, "label {label:?}");
        }
    }

    #[test]
    fn writes_on_red_in_a_color_that_can_be_read() {
        assert!(labeled_bar(90.0, "90%", RED).contains(&format!("38;5;{LIGHT_TEXT}m")));
        assert!(labeled_bar(10.0, "10%", GREEN).contains(&format!("38;5;{DARK_TEXT}m")));
    }

    #[test]
    fn spells_a_countdown_with_its_hours() {
        assert_eq!(plain(&countdown(2 * 3600 + 41 * 60)), "2h 41m");
        // Zero hours are spelled out too, or the label changes width mid-session.
        assert_eq!(plain(&countdown(59 * 60)), "0h 59m");
    }

    #[test]
    fn rounds_a_percentage_the_way_the_label_does() {
        assert_eq!(percent_label(0.4), "0%");
        assert_eq!(percent_label(42.6), "43%");
        assert_eq!(percent_label(100.0), "100%");
    }
}
