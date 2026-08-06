//! Drawing the panels. What the device does with the picture is the business of
//! divoomkit; what goes on the picture is ours.
//!
//! One screen, one panel: [`draw`] is handed what the screen was given in the
//! config and picks the drawing accordingly.

mod limits;
mod renewal;

use std::time::Duration;

use divoomkit::{BarStyle, Canvas, Frame, Rgb, Size};

use crate::i18n::{t, tf};
use crate::panel::config::Panel;
use crate::panel::usage::Snapshot;

/// The color of the Claude sparkle in the header.
const CLAUDE: Rgb = Rgb(0xd9, 0x77, 0x57);

/// The header with the sparkle on top, the rows below it.
const LABEL_X: i32 = 6;
const LABEL_WIDE: i32 = 22;
const BAR_X: i32 = 32;
const BAR_WIDTH: u32 = 128 - BAR_X as u32 - LABEL_X as u32;
const BAR_HEIGHT: u32 = 20;
const ROW_GAP: i32 = 7;
const HEADER_Y: i32 = 8;
const SPARK_SIZE: i32 = 17;
const FIRST_ROW_Y: i32 = 38;
const RESET_ICON: i32 = 15;

const GLYPH_HEIGHT: i32 = 7;

/// Everything a panel is drawn from. The billing cycle is only known once the
/// user has told us the day of the charge.
pub struct State<'a> {
    pub usage: &'a Snapshot,
    pub cycle: Option<crate::panel::billing::Cycle>,
}

pub fn draw(panel: Panel, state: &State) -> Result<Frame, String> {
    let mut canvas = Canvas::new(Size::Px128);
    header(&mut canvas, state.usage);

    // A panel drawn from the snapshot has nothing to say without one; the
    // renewal panel is counted from the calendar and does not care.
    if state.usage.trouble.is_some() && panel != Panel::Renewal {
        canvas.text_centered(52, 3, Rgb::GREY, t("NO"));
        canvas.text_centered(86, 2, Rgb::GREY, t("DATA"));
        return finish(canvas);
    }

    match panel {
        Panel::Limits => limits::draw(&mut canvas, state.usage),
        Panel::FiveHour => limits::draw_one(&mut canvas, state.usage, "five_hour", "5H"),
        Panel::Week => limits::draw_one(&mut canvas, state.usage, "seven_day", "7D"),
        Panel::Renewal => renewal::draw(&mut canvas, state.cycle),
    }
    finish(canvas)
}

fn finish(canvas: Canvas) -> Result<Frame, String> {
    canvas
        .finish()
        .map_err(|err| tf!("could not draw the panel: {0}", err))
}

/// The sparkle, the word, and how old the numbers are. The same on every panel:
/// the screens hang side by side and a header that moved would read as a
/// different device.
fn header(canvas: &mut Canvas, state: &Snapshot) {
    sparkle(canvas, LABEL_X, HEADER_Y, SPARK_SIZE, CLAUDE);
    canvas.text(
        LABEL_X + SPARK_SIZE + 7,
        HEADER_Y + (SPARK_SIZE - GLYPH_HEIGHT * 2) / 2,
        2,
        Rgb::WHITE,
        "CLAUDE",
    );

    // The numbers only grow during an active Claude Code session: if the
    // snapshot has not been updated for a while, the percentages below describe
    // the past, and that has to be visible. Smaller than the header, or the mark
    // stands right against it and reads as part of the word — "CLAUDE2H".
    if state.stale {
        let label = age_label(state.age);
        let width = divoomkit::font::text_width(&label, 1) as i32;
        canvas.text(
            128 - LABEL_X - width,
            HEADER_Y + (SPARK_SIZE - GLYPH_HEIGHT) / 2,
            1,
            Rgb::GREY,
            &label,
        );
    }
}

fn bar(canvas: &mut Canvas, y: i32, height: u32, fraction: f32, fill: Rgb, label: &str) {
    canvas.bar(
        (BAR_X, y, BAR_WIDTH, height),
        fraction,
        BarStyle {
            fill,
            label,
            // Red is dark enough for black digits to drown in it, and the status
            // line writes on it in white too.
            label_on_fill: if fill == Rgb::RED {
                Rgb::WHITE
            } else {
                Rgb::BLACK
            },
            ..Default::default()
        },
    );
}

/// The label of a row, in the column to the left of the bars.
fn row_label(canvas: &mut Canvas, y: i32, height: u32, text: &str) {
    canvas.text(
        LABEL_X,
        y + (height as i32 - GLYPH_HEIGHT * 2) / 2,
        2,
        Rgb::GREY,
        text,
    );
}

fn age_label(age: Duration) -> String {
    let hours = age.as_secs() / 3600;
    if hours > 0 {
        return format!("{hours}H");
    }
    format!("{}M", age.as_secs() / 60)
}

/// "2:41" until the window resets. The status line spells the time with letters,
/// but in a 5×7 grid the Russian letter for an hour is indistinguishable from a
/// four, and the label reads as "24 41M". A colon can be mistaken for nothing
/// and needs no translation.
fn countdown_label(seconds: i64) -> String {
    if seconds <= 0 {
        return "-".into();
    }
    let minutes = seconds / 60;
    format!("{}:{:02}", minutes / 60, minutes % 60)
}

/// A petal is the wider the closer it is to the center: the axial rays have more
/// room, the diagonal ones are half as long — that keeps the sparkle from
/// turning into an even star.
fn sparkle(canvas: &mut Canvas, x: i32, y: i32, size: i32, color: Rgb) {
    let radius = size / 2;
    for dy in -radius..=radius {
        for dx in -radius..=radius {
            let (ax, ay) = ((dx.abs()) as f64, (dy.abs()) as f64);
            let distance = ((dx * dx + dy * dy) as f64).sqrt();
            if distance > radius as f64 {
                continue;
            }
            let axial = ax <= (radius as f64 - ay) / 2.6 || ay <= (radius as f64 - ax) / 2.6;
            let diagonal = (ax - ay).abs() <= (radius as f64 - ax.max(ay)) / 2.6
                && distance <= radius as f64 * 0.82;

            if axial || diagonal {
                canvas.pixel(x + radius + dx, y + radius + dy, color);
            }
        }
    }
}

fn reset_icon(canvas: &mut Canvas, x: i32, y: i32, size: i32, color: Rgb) {
    let radius = size as f64 / 2.0;
    let center = radius - 0.5;

    for dy in 0..size {
        for dx in 0..size {
            let (ox, oy) = (dx as f64 - center, dy as f64 - center);
            let distance = ox.hypot(oy);
            if distance > radius || distance < radius - 2.5 {
                continue;
            }
            // The ring is broken at the upper right — that is where the arrow
            // head starts.
            let angle = ox.atan2(-oy);
            if angle > 0.2 && angle < 1.5 {
                continue;
            }
            canvas.pixel(x + dx, y + dy, color);
        }
    }

    // The arrow head at the break: three rows narrowing to the right.
    let (tip_x, tip_y) = (x + size / 2, y);
    for row in 0..3 {
        canvas.rect((tip_x + row, tip_y + row, (3 - row) as u32, 1), color);
        canvas.rect((tip_x + row, tip_y - row, (3 - row) as u32, 1), color);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::i18n::{LANGS, translate};
    use crate::panel::billing;
    use crate::panel::usage;

    fn state(usage: &Snapshot) -> State<'_> {
        State { usage, cycle: None }
    }

    #[test]
    fn spells_the_countdown_in_digits() {
        assert_eq!(countdown_label(2 * 3600 + 41 * 60), "2:41");
        assert_eq!(countdown_label(59 * 60), "0:59");
        assert_eq!(countdown_label(0), "-");
    }

    #[test]
    fn tells_how_old_the_numbers_are() {
        assert_eq!(age_label(Duration::from_secs(3 * 3600)), "3H");
        assert_eq!(age_label(Duration::from_secs(20 * 60)), "20M");
        assert_eq!(age_label(Duration::from_secs(90)), "1M");
    }

    #[test]
    fn every_label_of_the_panel_has_a_glyph_in_every_language() {
        // An unknown character is drawn as "?" — a panel of question marks is
        // worse than an untranslated word.
        for lang in LANGS {
            for label in ["RESET", "NO", "DATA", "DATE", "RENEWS", "TODAY"] {
                let drawn = translate(*lang, label);
                assert!(
                    divoomkit::font::covers(drawn),
                    "{drawn:?} has no glyph in {lang:?}"
                );
            }
        }
    }

    #[test]
    fn every_label_of_the_panel_fits_its_bar() {
        for lang in LANGS {
            let drawn = translate(*lang, "RESET");
            assert!(
                divoomkit::font::text_width(drawn, 2) <= BAR_WIDTH,
                "{drawn:?} does not fit the bar"
            );
        }
    }

    #[test]
    fn draws_a_panel_of_the_size_the_device_takes() {
        for panel in Panel::ALL {
            let frame = draw(*panel, &state(&Snapshot::default())).unwrap();
            assert_eq!(&frame.bytes()[..6], b"GIF89a");
            // 128 pixels, little-endian, right after the header.
            assert_eq!(&frame.bytes()[6..10], &[128, 0, 128, 0], "{panel:?}");
        }
    }

    #[test]
    fn draws_something_else_when_there_are_no_numbers() {
        let empty = draw(Panel::Limits, &state(&Snapshot::default())).unwrap();
        let numbers = usage::for_test(42.0, 9000, 400_000);
        let with_numbers = draw(Panel::Limits, &state(&numbers)).unwrap();
        assert_ne!(empty.hash(), with_numbers.hash());
    }

    #[test]
    fn tells_the_panels_apart() {
        let numbers = usage::for_test(42.0, 9000, 400_000);
        let mut drawn: Vec<String> = Panel::ALL
            .iter()
            .map(|panel| {
                draw(
                    *panel,
                    &State {
                        usage: &numbers,
                        cycle: Some(billing::cycle(20, 1_786_000_000.0)),
                    },
                )
                .unwrap()
                .hash()
                .to_string()
            })
            .collect();
        let count = drawn.len();
        drawn.sort();
        drawn.dedup();
        assert_eq!(count, drawn.len(), "two panels draw the same picture");
    }

    #[test]
    fn still_counts_the_days_when_the_snapshot_is_gone() {
        // The renewal panel is calendar arithmetic: a missing snapshot is no
        // reason for it to show NO DATA.
        let blank = usage::trouble_for_test();
        let counted = draw(
            Panel::Renewal,
            &State {
                usage: &blank,
                cycle: Some(billing::cycle(20, 1_786_000_000.0)),
            },
        )
        .unwrap();
        let nothing = draw(Panel::Limits, &state(&blank)).unwrap();
        assert_ne!(counted.hash(), nothing.hash());
    }
}
