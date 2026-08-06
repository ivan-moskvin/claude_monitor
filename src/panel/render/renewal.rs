//! The panel that counts the days to the next charge.

use divoomkit::{BarStyle, Canvas, Rgb};

use super::{BAR_WIDTH, BAR_X};
use crate::i18n::t;
use crate::panel::billing::Cycle;

const CAPTION_Y: i32 = 36;
const COUNT_Y: i32 = 56;
const BAR_Y: i32 = 106;
const BAR_HEIGHT: u32 = 14;

/// The day of the charge itself, and the days just before it: the subscription
/// is about to renew, which is good news and not a warning.
const DUE_SOON: i64 = 2;

pub fn draw(canvas: &mut Canvas, cycle: Option<Cycle>) {
    let Some(cycle) = cycle else {
        // Nothing tells us the billing day — it is asked for once and kept in
        // the config, and until then there is nothing to count.
        canvas.text_centered(52, 3, Rgb::GREY, t("NO"));
        canvas.text_centered(86, 2, Rgb::GREY, t("DATE"));
        return;
    };

    canvas.text_centered(CAPTION_Y, 2, Rgb::GREY, t("RENEWS"));

    let color = if cycle.days_left <= DUE_SOON {
        Rgb::GREEN
    } else {
        // Cyan is the color of time on this device — the reset bar of the
        // limits panel uses it for the same reason: it shows a clock, not a
        // risk.
        Rgb::CYAN
    };

    if cycle.days_left == 0 {
        // The word is as wide as the screen allows and no wider: five letters
        // at the size of the digits would run off both edges.
        canvas.text_centered(COUNT_Y + 8, 3, color, t("TODAY"));
    } else {
        canvas.text_centered(COUNT_Y, digit_scale(cycle.days_left), color, &count(cycle));
    }

    canvas.bar(
        (BAR_X, BAR_Y, BAR_WIDTH, BAR_HEIGHT),
        cycle.elapsed_fraction(),
        BarStyle {
            fill: Rgb::GREY,
            label: &date_label(cycle),
            label_on_fill: Rgb::BLACK,
            ..Default::default()
        },
    );
}

fn count(cycle: Cycle) -> String {
    cycle.days_left.to_string()
}

/// Two digits are what a month gives; a three-digit count would only appear on a
/// config edited by hand, and it still has to fit.
fn digit_scale(days: i64) -> u32 {
    match days {
        0..=99 => 6,
        _ => 4,
    }
}

/// The day of the charge, written the short way. A month has no glyph problem
/// in digits, and the order — day then month — is the one the device shows in
/// its own clock faces.
fn date_label(cycle: Cycle) -> String {
    format!("{:02}.{:02}", cycle.renews_on.day, cycle.renews_on.month)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::civil::days_from_civil;
    use crate::panel::billing;
    use crate::panel::config::Panel;
    use crate::panel::render::{State, draw as draw_panel};
    use crate::panel::usage::Snapshot;

    fn at(year: i64, month: u32, day: u32) -> f64 {
        days_from_civil(year, month as i64, day as i64) as f64 * 86_400.0 + 43_200.0
    }

    fn frame(cycle: Option<billing::Cycle>) -> String {
        let usage = Snapshot::default();
        draw_panel(
            Panel::Renewal,
            &State {
                usage: &usage,
                cycle,
            },
        )
        .unwrap()
        .hash()
        .to_string()
    }

    #[test]
    fn draws_a_different_picture_as_the_days_run_out() {
        let far = frame(Some(billing::cycle(20, at(2026, 8, 1))));
        let near = frame(Some(billing::cycle(20, at(2026, 8, 18))));
        let today = frame(Some(billing::cycle(20, at(2026, 8, 20))));

        assert_ne!(far, near);
        assert_ne!(near, today);
    }

    #[test]
    fn says_so_when_there_is_no_billing_day() {
        assert_ne!(
            frame(None),
            frame(Some(billing::cycle(20, at(2026, 8, 1)))),
            "a panel with nothing to count must not look like one that counts"
        );
    }

    #[test]
    fn writes_the_day_of_the_charge_the_short_way() {
        assert_eq!(date_label(billing::cycle(20, at(2026, 8, 1))), "20.08");
        assert_eq!(date_label(billing::cycle(5, at(2026, 12, 20))), "05.01");
    }

    #[test]
    fn the_date_fits_the_bar_it_is_written_on() {
        let widest = date_label(billing::cycle(30, at(2026, 11, 1)));
        assert!(
            divoomkit::font::text_width(&widest, 2) <= BAR_WIDTH,
            "{widest:?} does not fit"
        );
    }

    #[test]
    fn the_words_fit_the_screen_in_every_language() {
        use crate::i18n::{LANGS, translate};

        // These are written across the whole screen rather than inside a bar,
        // and the Russian words are the longer ones.
        for lang in LANGS {
            for (label, scale) in [("RENEWS", 2), ("TODAY", 3), ("DATE", 2)] {
                let drawn = translate(*lang, label);
                let width = divoomkit::font::text_width(drawn, scale);
                assert!(width <= 128, "{drawn:?} is {width} wide at scale {scale}");
            }
        }
    }

    #[test]
    fn the_count_fits_the_screen_at_every_width() {
        for days in [1i64, 9, 31, 99] {
            let text = days.to_string();
            let width = divoomkit::font::text_width(&text, digit_scale(days));
            assert!(width <= 128, "{days} days is {width} wide");
        }
    }
}
