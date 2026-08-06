//! The panels drawn from the limits Claude Code reports.

use divoomkit::{BarStyle, Canvas, Rgb};

use super::{
    BAR_HEIGHT, BAR_WIDTH, BAR_X, FIRST_ROW_Y, LABEL_WIDE, LABEL_X, RESET_ICON, ROW_GAP, bar,
    countdown_label, reset_icon, row_label,
};
use crate::i18n::t;
use crate::panel::usage::{FIVE_HOUR_SECONDS, SEVEN_DAY_SECONDS, Snapshot, Window};

/// The week row is split in two: the usage above, the week running out below,
/// together taking the height of an ordinary row.
const WEEK_BAR_HEIGHT: u32 = 14;
const WEEK_GAP: i32 = 2;
const WEEK_ELAPSED_HEIGHT: u32 = BAR_HEIGHT - WEEK_BAR_HEIGHT - WEEK_GAP as u32;

/// All three bars: the five-hour window, the time to its reset, the week.
pub fn draw(canvas: &mut Canvas, state: &Snapshot) {
    let (five, week) = (state.window("five_hour"), state.window("seven_day"));

    let mut y = FIRST_ROW_Y;
    row_label(canvas, y, BAR_HEIGHT, "5H");
    bar(
        canvas,
        y,
        BAR_HEIGHT,
        five.fraction(),
        five.tint(),
        &five.percent_label(),
    );

    y += BAR_HEIGHT as i32 + ROW_GAP;
    reset_icon(
        canvas,
        LABEL_X + (LABEL_WIDE - RESET_ICON) / 2,
        y + (BAR_HEIGHT as i32 - RESET_ICON) / 2,
        RESET_ICON,
        Rgb::GREY,
    );
    bar(
        canvas,
        y,
        BAR_HEIGHT,
        five.elapsed_fraction(FIVE_HOUR_SECONDS),
        reset_tint(five),
        &reset_label(five),
    );

    y += BAR_HEIGHT as i32 + ROW_GAP;
    row_label(canvas, y, BAR_HEIGHT, "7D");
    week_row(canvas, y, week);
}

/// One window, given the whole picture: the percentage large enough to read
/// across a room, the window running out beneath it.
pub fn draw_one(canvas: &mut Canvas, state: &Snapshot, id: &str, label: &str) {
    let window = state.window(id);
    let seconds = if id == "five_hour" {
        FIVE_HOUR_SECONDS
    } else {
        SEVEN_DAY_SECONDS
    };

    canvas.text_centered(44, 2, Rgb::GREY, label);

    let percent = window.percent_label();
    let scale = if percent.chars().count() > 3 { 4 } else { 5 };
    canvas.text_centered(60, scale, window.tint(), &percent);

    canvas.bar(
        (BAR_X, 104, BAR_WIDTH, 10),
        window.elapsed_fraction(seconds),
        BarStyle {
            fill: Rgb::GREY,
            ..Default::default()
        },
    );
}

/// The grey strip below the week is the week itself running out. Usage ahead of
/// it is spent faster than the window gives it back; behind it there is room to
/// spare. Without a resets_at there is no such strip to draw, and the usage
/// takes the whole row.
fn week_row(canvas: &mut Canvas, y: i32, week: Window) {
    if !week.timed() {
        bar(
            canvas,
            y,
            BAR_HEIGHT,
            week.fraction(),
            week.tint(),
            &week.percent_label(),
        );
        return;
    }

    bar(
        canvas,
        y,
        WEEK_BAR_HEIGHT,
        week.fraction(),
        week.tint(),
        &week.percent_label(),
    );
    canvas.bar(
        (
            BAR_X,
            y + (WEEK_BAR_HEIGHT as i32) + WEEK_GAP,
            BAR_WIDTH,
            WEEK_ELAPSED_HEIGHT,
        ),
        week.elapsed_fraction(SEVEN_DAY_SECONDS),
        BarStyle {
            fill: Rgb::GREY,
            ..Default::default()
        },
    );
}

fn reset_label(five: Window) -> String {
    if five.expired {
        return t("RESET").into();
    }
    countdown_label(five.seconds_left)
}

fn reset_tint(five: Window) -> Rgb {
    if five.expired { Rgb::GREY } else { Rgb::CYAN }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::panel::config::Panel;
    use crate::panel::render::{GLYPH_HEIGHT, State, draw as draw_panel};
    use crate::panel::usage;

    fn frame(panel: Panel, usage: &Snapshot) -> String {
        draw_panel(panel, &State { usage, cycle: None })
            .unwrap()
            .hash()
            .to_string()
    }

    #[test]
    fn shows_how_much_of_the_week_has_passed() {
        let day = 24 * 3600;
        assert_ne!(
            frame(Panel::Limits, &usage::for_test(42.0, 9000, 6 * day)),
            frame(Panel::Limits, &usage::for_test(42.0, 9000, day)),
            "the same usage at a different point of the week"
        );
    }

    #[test]
    fn the_week_row_stays_within_its_row() {
        assert_eq!(
            WEEK_BAR_HEIGHT + WEEK_GAP as u32 + WEEK_ELAPSED_HEIGHT,
            BAR_HEIGHT
        );
        // The percentage is written at scale 2 and must still fit the shortened bar.
        assert!(WEEK_BAR_HEIGHT >= (GLYPH_HEIGHT * 2) as u32);
    }

    #[test]
    fn a_single_window_panel_follows_its_own_window() {
        let quiet = usage::for_test(4.0, 9000, 400_000);
        let busy = usage::for_test(91.0, 9000, 400_000);
        assert_ne!(
            frame(Panel::FiveHour, &quiet),
            frame(Panel::FiveHour, &busy)
        );
        assert_ne!(frame(Panel::Week, &quiet), frame(Panel::Week, &busy));
    }

    #[test]
    fn a_percentage_of_any_width_fits_the_screen() {
        for used in [0.0, 7.0, 42.0, 100.0] {
            let state = usage::for_test(used, 9000, 400_000);
            let mut canvas = divoomkit::Canvas::new(divoomkit::Size::Px128);
            draw_one(&mut canvas, &state, "five_hour", "5H");
            // The canvas clips, so what this really checks is that the label was
            // scaled to something the drawing accepts at all.
            assert!(canvas.finish().is_ok(), "{used}%");
        }
    }
}
