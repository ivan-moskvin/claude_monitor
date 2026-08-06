//! Calendar arithmetic, by Howard Hinnant's civil algorithms.
//!
//! Three places need it and none of them may pull a date crate in: the snapshot
//! stamps itself, the panel reads that stamp back, and the billing cycle counts
//! the days to the next renewal.

/// A calendar date to days since the epoch.
pub fn days_from_civil(year: i64, month: i64, day: i64) -> i64 {
    let year = if month <= 2 { year - 1 } else { year };
    let era = year.div_euclid(400);
    let year_of_era = year - era * 400;
    let shifted = if month > 2 { month - 3 } else { month + 9 };
    let day_of_year = (153 * shifted + 2) / 5 + day - 1;
    let day_of_era = year_of_era * 365 + year_of_era / 4 - year_of_era / 100 + day_of_year;
    era * 146_097 + day_of_era - 719_468
}

/// Days since the epoch to a calendar date.
pub fn civil_from_days(days: i64) -> (i64, u32, u32) {
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

/// How many days the month has. The billing day is a number the user gave us
/// once, and not every month has a 31st to put it on.
pub fn days_in_month(year: i64, month: u32) -> u32 {
    match month {
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        4 | 6 | 9 | 11 => 30,
        _ if is_leap(year) => 29,
        _ => 28,
    }
}

fn is_leap(year: i64) -> bool {
    year % 4 == 0 && (year % 100 != 0 || year % 400 == 0)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn turns_days_into_a_calendar_date() {
        assert_eq!(civil_from_days(0), (1970, 1, 1));
        assert_eq!(civil_from_days(19_723), (2024, 1, 1));
        // A leap day, which is where a home-made calendar usually goes wrong.
        assert_eq!(civil_from_days(19_782), (2024, 2, 29));
        assert_eq!(civil_from_days(20_608), (2026, 6, 4));
    }

    #[test]
    fn turns_a_date_back_into_days() {
        for days in [0, 19_723, 19_782, 20_608, -1, 30_000] {
            let (year, month, day) = civil_from_days(days);
            assert_eq!(
                days_from_civil(year, month as i64, day as i64),
                days,
                "{year:04}-{month:02}-{day:02}"
            );
        }
    }

    #[test]
    fn counts_the_days_of_a_month() {
        assert_eq!(days_in_month(2026, 1), 31);
        assert_eq!(days_in_month(2026, 2), 28);
        assert_eq!(days_in_month(2024, 2), 29, "a leap year");
        assert_eq!(days_in_month(2000, 2), 29, "a leap century");
        assert_eq!(days_in_month(1900, 2), 28, "a century that is not");
        assert_eq!(days_in_month(2026, 4), 30);
    }
}
