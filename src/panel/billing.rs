//! The subscription cycle: how long until the next renewal.
//!
//! Nothing tells us this. Claude Code hands over `rate_limits` and nothing else
//! — the `seven_day` window is a rolling one and slides forward as it is spent,
//! so it says nothing about the day of the charge. The day of the month comes
//! from the user once and the calendar does the rest.

use crate::civil::{civil_from_days, days_from_civil, days_in_month};

/// Where in the billing period the moment is.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Cycle {
    /// Whole days until the renewal. The day of the charge itself is zero.
    pub days_left: i64,
    /// The length of the period the renewal ends, in days: a month is not a
    /// fixed thing and the bar has to be drawn against the right one.
    pub period_days: i64,
    pub renews_on: Date,
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Date {
    pub year: i64,
    pub month: u32,
    pub day: u32,
}

/// The cycle a billing day stands for at this moment. Takes the moment as
/// seconds since the epoch so that a test can say when it is.
pub fn cycle(billing_day: u8, now: f64) -> Cycle {
    let today = (now / 86_400.0).floor() as i64;
    let (year, month, _) = civil_from_days(today);

    // The charge of this month, then of the next one: whichever has not passed
    // yet is the one being waited for. A charge that falls today is still ahead
    // — the day of the renewal reads as zero days left, not as a month.
    let this_month = charge(year, month, billing_day);
    let next = if this_month >= today {
        this_month
    } else {
        let (year, month) = next_month(year, month);
        charge(year, month, billing_day)
    };

    let previous = previous_charge(next, billing_day);
    let (year, month, day) = civil_from_days(next);

    Cycle {
        days_left: next - today,
        period_days: (next - previous).max(1),
        renews_on: Date { year, month, day },
    }
}

impl Cycle {
    /// How much of the period is behind us, for the bar.
    pub fn elapsed_fraction(&self) -> f32 {
        let left = self.days_left.clamp(0, self.period_days) as f32;
        1.0 - left / self.period_days as f32
    }
}

/// The day of the charge in a given month, moved back to the last day when the
/// month is too short for it: a subscription taken out on the 31st is charged
/// on the 28th of February, not on the 3rd of March.
fn charge(year: i64, month: u32, billing_day: u8) -> i64 {
    let day = (billing_day as u32).clamp(1, days_in_month(year, month));
    days_from_civil(year, month as i64, day as i64)
}

/// The charge that opened the period this one closes. Counted rather than
/// assumed to be thirty days: February would draw a bar that never fills.
fn previous_charge(next: i64, billing_day: u8) -> i64 {
    let (year, month, _) = civil_from_days(next);
    let (year, month) = previous_month(year, month);
    charge(year, month, billing_day)
}

fn next_month(year: i64, month: u32) -> (i64, u32) {
    if month == 12 {
        (year + 1, 1)
    } else {
        (year, month + 1)
    }
}

fn previous_month(year: i64, month: u32) -> (i64, u32) {
    if month == 1 {
        (year - 1, 12)
    } else {
        (year, month - 1)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Midday of a given date — the hour must not decide anything.
    fn at(year: i64, month: u32, day: u32) -> f64 {
        days_from_civil(year, month as i64, day as i64) as f64 * 86_400.0 + 43_200.0
    }

    #[test]
    fn counts_the_days_to_the_next_charge() {
        let cycle = cycle(20, at(2026, 8, 6));
        assert_eq!(cycle.days_left, 14);
        assert_eq!(
            cycle.renews_on,
            Date {
                year: 2026,
                month: 8,
                day: 20
            }
        );
    }

    #[test]
    fn rolls_over_into_the_next_month_once_the_day_has_passed() {
        let cycle = cycle(20, at(2026, 8, 21));
        assert_eq!(cycle.days_left, 30);
        assert_eq!(cycle.renews_on.month, 9);
    }

    #[test]
    fn reads_the_day_of_the_charge_as_no_days_left() {
        let cycle = cycle(20, at(2026, 8, 20));
        assert_eq!(cycle.days_left, 0);
        assert_eq!(cycle.renews_on.day, 20);
        assert_eq!(cycle.elapsed_fraction(), 1.0);
    }

    #[test]
    fn moves_a_day_the_month_is_too_short_for() {
        // The 31st in February: the charge is on the last day the month has.
        let short = cycle(31, at(2026, 2, 10));
        assert_eq!(
            short.renews_on,
            Date {
                year: 2026,
                month: 2,
                day: 28
            }
        );
        assert_eq!(short.days_left, 18);

        let leap = cycle(31, at(2024, 2, 10));
        assert_eq!(leap.renews_on.day, 29);
    }

    #[test]
    fn crosses_the_new_year() {
        let cycle = cycle(5, at(2026, 12, 20));
        assert_eq!(
            cycle.renews_on,
            Date {
                year: 2027,
                month: 1,
                day: 5
            }
        );
        assert_eq!(cycle.days_left, 16);
    }

    #[test]
    fn measures_the_period_by_the_calendar_and_not_by_thirty() {
        // February to March: the period is as long as February is.
        let february = cycle(1, at(2026, 2, 10));
        assert_eq!(february.period_days, 28);

        let july = cycle(1, at(2026, 7, 10));
        assert_eq!(july.period_days, 31, "the 1st of July to the 1st of August");
    }

    #[test]
    fn fills_the_bar_as_the_period_runs_out() {
        let fresh = cycle(20, at(2026, 7, 21));
        assert!(fresh.elapsed_fraction() < 0.05, "the day after the charge");

        let halfway = cycle(20, at(2026, 8, 5));
        assert!((halfway.elapsed_fraction() - 0.5).abs() < 0.05);

        let due = cycle(20, at(2026, 8, 19));
        assert!(due.elapsed_fraction() > 0.95);
    }

    #[test]
    fn takes_a_day_outside_the_calendar_as_the_nearest_one_inside_it() {
        // The wizard will not write a zero, but a config edited by hand might.
        assert_eq!(cycle(0, at(2026, 8, 6)).renews_on.day, 1);
        assert_eq!(cycle(99, at(2026, 8, 6)).renews_on.day, 31);
    }
}
