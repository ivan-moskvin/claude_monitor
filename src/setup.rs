//! The wizard: everything that has to be decided once, asked in one place.
//!
//! It is not an onboarding step but the settings themselves — it may be run at
//! any time, over a setup that is already working, and it starts every question
//! on the answer that is in force now. Walking through it with Enter changes
//! nothing.
//!
//! Nothing is typed: the questions are lists and the arrow keys move through
//! them. The one exception is the day of the charge, which is a number and is
//! quicker typed than scrolled to.
//!
//! Applying the answers is the delicate part: the bridge holds the screens, so
//! it is stopped, the screens that are being given up get their clock faces
//! back, and only then is the config written and the bridge started again.

use std::io::{IsTerminal, Write};

use divoomkit::{Discover, discover};

use crate::i18n::{t, tf};
use crate::panel::config::{Config, Missing, Panel, Screen};
use crate::panel::{self, daemon};
use crate::{Outcome, install, menu};

pub fn run() -> Outcome {
    if !std::io::stdin().is_terminal() {
        return Err(t("setup asks questions and needs a terminal").into());
    }

    let mut config = match Config::load() {
        Ok(config) => config,
        Err(Missing::NeverTurnedOn) => Config::fresh(),
        Err(broken) => return Err(broken.to_string()),
    };
    let before = config.screens.clone();

    println!(
        "{}",
        t("claudestatus setup — run it again whenever you like.\n")
    );

    status_line()?;

    if !device(&mut config)? {
        return Ok(());
    }
    screens(&mut config)?;
    billing_day(&mut config)?;

    apply(config, &before)
}

/// The status line is what the whole thing is for; the panels are the extra.
fn status_line() -> Outcome {
    if install::registered()? {
        println!("{}", t("The status line is registered with Claude Code."));
        return Ok(());
    }
    if !menu::confirm(t("Register the status line in Claude Code?"), true)? {
        return Ok(());
    }
    install::install()
}

/// Which device gets the panels. Answers whether there is one at all: without a
/// device there are no screens to ask about.
fn device(config: &mut Config) -> Result<bool, String> {
    let has_device = !config.ip.is_empty();
    if has_device {
        println!(
            "{}",
            tf!("The panels go to {0} ({1}).", config.name, config.ip)
        );
        if !menu::confirm(t("Look for the device again?"), false)? {
            return Ok(true);
        }
    } else if !menu::confirm(t("Show the panels on a Divoom Times Gate?"), true)? {
        return Ok(false);
    }

    println!("{}", t("Looking for Divoom devices…"));
    let found = match discover(Discover::new()) {
        Ok(found) if !found.is_empty() => found,
        Ok(_) | Err(_) => {
            println!("{}", t("No Divoom devices are visible on this network."));
            // A device that is merely switched off must not cost the user the
            // screens they have already chosen.
            return Ok(has_device);
        }
    };

    let picked = choose_device(&found, config)?;
    println!(
        "{}",
        tf!("Device: {0} — {1}", panel::label(&picked), picked.ip())
    );
    panel::adopt(config, &picked);
    Ok(true)
}

/// One device needs no choosing; several are a list like every other question.
fn choose_device(
    found: &[divoomkit::Device],
    config: &Config,
) -> Result<divoomkit::Device, String> {
    if found.len() == 1 {
        return Ok(found[0].clone());
    }
    let known = panel::pick_known(found, config);
    let at = known
        .as_ref()
        .and_then(|known| found.iter().position(|device| device.ip() == known.ip()))
        .unwrap_or(0);

    let items: Vec<String> = found
        .iter()
        .map(|device| format!("{} — {}", panel::label(device), device.ip()))
        .collect();

    match menu::select(t("Which one gets the panels?"), &items, at)? {
        Some(picked) => Ok(found[picked].clone()),
        None => Err(t("nothing was chosen").into()),
    }
}

/// What goes on each of the five screens, one screen at a time.
fn screens(config: &mut Config) -> Outcome {
    // "nothing" first: the answer for a screen the wizard must not touch is the
    // one most screens need, and it starts the list.
    let mut items = vec![t("nothing — leave the screen alone").to_string()];
    items.extend(Panel::ALL.iter().map(|panel| panel.title().to_string()));

    println!("{}", t("\nWhat goes on which screen:"));

    for index in 0..divoomkit::SCREEN_COUNT {
        let current = config.screen(index).map(|screen| screen.panel);
        let at = match current {
            None => 0,
            Some(panel) => {
                Panel::ALL
                    .iter()
                    .position(|known| *known == panel)
                    .unwrap_or(0)
                    + 1
            }
        };

        let title = tf!("Screen {0}", index + 1);
        let Some(picked) = menu::select(&title, &items, at)? else {
            // Cancelled: what was answered so far is not written down, and the
            // panels keep running as they were.
            return Err(t("setup was cancelled, nothing changed").into());
        };
        config.set_screen(index, (picked > 0).then(|| Panel::ALL[picked - 1]));
    }

    if config.screens.is_empty() {
        println!("{}", t("No screen has a panel on it."));
    }
    Ok(())
}

/// The day of the charge — asked only when a screen is going to show it, and
/// only when we do not know it yet. A number is typed: scrolling to the 27th of
/// a list of thirty-one is worse than pressing two keys.
fn billing_day(config: &mut Config) -> Outcome {
    let wanted = config
        .screens
        .iter()
        .any(|screen| screen.panel == Panel::Renewal);
    if !wanted {
        return Ok(());
    }

    let shown = match config.billing_day {
        Some(day) => day.to_string(),
        None => t("not set").to_string(),
    };
    println!(
        "{}",
        t("\nNothing reports the billing date — Claude Code gives out the rolling windows only.")
    );

    loop {
        print!(
            "{}",
            tf!(
                "Which day of the month is the subscription charged on? 1–31 [{0}]: ",
                shown
            )
        );
        let _ = std::io::stdout().flush();

        let mut answer = String::new();
        std::io::stdin()
            .read_line(&mut answer)
            .map_err(|err| err.to_string())?;
        let answer = answer.trim();

        if answer.is_empty() {
            if config.billing_day.is_some() {
                return Ok(());
            }
            println!(
                "{}",
                t("Without it the screen would have nothing to count.")
            );
            continue;
        }
        match answer.parse::<u8>() {
            Ok(day) if (1..=31).contains(&day) => {
                config.billing_day = Some(day);
                return Ok(());
            }
            _ => println!("{}", t("A number from 1 to 31, please.")),
        }
    }
}

/// Writes the answers down and moves the panels to where they now belong.
fn apply(mut config: Config, before: &[Screen]) -> Outcome {
    if daemon::running() {
        // The bridge holds the screens and gives them back on its way out;
        // stopping it first is what makes a screen safe to hand over.
        daemon::stop();
    }

    // A screen we are giving up would otherwise keep the last frame and go into
    // endless loading. It is restored from what it was before we took it, which
    // only the old config knows.
    let dropped: Vec<Screen> = before
        .iter()
        .filter(|screen| config.screen(screen.index).is_none())
        .cloned()
        .collect();
    if !dropped.is_empty() {
        daemon::restore_screens(&config, &dropped);
    }

    config.on = Some(!config.screens.is_empty() && !config.ip.is_empty());
    config.save()?;

    if !config.enabled() {
        println!("{}", t("\nSaved. No panels are running."));
        return Ok(());
    }

    daemon::ensure_running();
    println!("{}", t("\nSaved."));
    for screen in config.live_screens() {
        println!(
            "  {} {}",
            tf!("Screen {0}:", screen.index + 1),
            screen.panel.title()
        );
    }
    // A renewal screen with no billing day is the one case where a chosen panel
    // is not run, and silence about it would look like a bug.
    for screen in &config.screens {
        if !screen.panel.ready(&config) {
            println!(
                "{}",
                tf!(
                    "Screen {0} is waiting for the billing day — run setup again to give it one.",
                    screen.index + 1
                )
            );
        }
    }
    Ok(())
}
