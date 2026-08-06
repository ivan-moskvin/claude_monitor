//! The `divoom` subcommand: the Claude panels on the screens of a Divoom Times
//! Gate.
//!
//! Everything about the device — finding it, serving it frames, giving a screen
//! back — belongs to divoomkit. What is here is ours alone: which device was
//! chosen, what goes on which screen, and the fact that nobody starts the
//! bridge by hand.
//!
//! A screen is held by a bridge of its own, on a port of its own, in a thread of
//! its own. The firmware takes them one at a time — `Device/PlayGif` names the
//! screen it is meant for and leaves the neighbours alone — so several panels
//! live on one device without knowing about each other.

pub mod billing;
pub mod config;
pub mod daemon;
mod render;
pub mod usage;

use std::time::{Duration, SystemTime, UNIX_EPOCH};

use divoomkit::{Bridge, ClockFace, Device, Discover, Error, Pacing, Tick, discover};

use crate::Outcome;
use crate::i18n::{t, tf};
use config::{Config, Missing, Panel, Screen};

pub use daemon::{ensure_running, stop};

const USAGE: &str = "claudestatus divoom — the Claude panels on a Divoom Times Gate.

Usage:
  claudestatus divoom on           turn the panels back on
  claudestatus divoom off          turn them off and give the screens their clock faces back
  claudestatus divoom              keep the panels updated (works while running)
  claudestatus divoom once         send every panel once and exit
  claudestatus divoom preview FILE save a frame to a file without touching the device

Which device and what goes on which screen is asked by claudestatus setup.
";

/// The snapshot is rewritten on every call of the status line, that is often:
/// poll briskly, reading a file costs nothing.
const POLL: Duration = Duration::from_secs(5);

/// The device was switched off — the bridge does not hang around forever: it
/// exits, and the status line starts it again once the Times Gate is back.
const GIVE_UP_AFTER: Duration = Duration::from_secs(10 * 60);

pub fn run(args: &[String]) -> Outcome {
    match args.first().map(String::as_str) {
        None => bridge(false),
        Some("once") => bridge(true),
        Some("on") => turn_on(),
        Some("off") => turn_off(),
        Some("preview") => preview(args.get(1)),
        Some("help" | "--help" | "-h") => {
            print!("{}", t(USAGE));
            Ok(())
        }
        Some(other) => Err(tf!("unknown command: {0}\n\n{1}", other, t(USAGE))),
    }
}

/// What the panels are drawn from right now. Read once per tick and shared by
/// every screen: they all describe the same moment, and reading the snapshot
/// once per screen would let two panels disagree.
fn state() -> (usage::Snapshot, Option<billing::Cycle>) {
    let cycle = Config::load()
        .ok()
        .and_then(|config| config.billing_day)
        .map(|day| billing::cycle(day, now()));
    (usage::read(), cycle)
}

fn now() -> f64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|since| since.as_secs_f64())
        .unwrap_or(0.0)
}

fn preview(file: Option<&String>) -> Outcome {
    let Some(file) = file else {
        return Err(t("name a file: claudestatus divoom preview panel.gif").into());
    };
    // The panel of the first screen, or the limits when no screen was ever
    // chosen: a preview is for looking at the drawing, not at the config.
    let panel = Config::load()
        .ok()
        .and_then(|config| config.screens.first().map(|screen| screen.panel))
        .unwrap_or(Panel::Limits);

    let (usage, cycle) = state();
    let frame = render::draw(
        panel,
        &render::State {
            usage: &usage,
            cycle,
        },
    )?;
    std::fs::write(file, frame.bytes()).map_err(|err| tf!("could not write {0}: {1}", file, err))
}

/// Keeps every screen updated, or sends each one once and leaves.
fn bridge(once: bool) -> Outcome {
    let mut config = Config::load().map_err(|err| err.to_string())?;
    if !config.enabled() {
        return Err(Missing::NeverTurnedOn.to_string());
    }
    let screens = config.live_screens();
    if screens.is_empty() {
        return Err(t("no screen has a panel on it — claudestatus setup").into());
    }

    // The device is looked up before every run: its address comes from DHCP and
    // does not have to be the one of the last time.
    let device = locate(&mut config)?;

    let mut bridges = Vec::new();
    for screen in &screens {
        bridges.push((screen.clone(), build(&config, &device, screen)?));
    }

    // The clock faces are written down from the main thread and all at once: a
    // Config::update from each bridge would have them overwrite one another.
    remember_clock_faces(&bridges)?;

    if once {
        let (usage, cycle) = state();
        for (screen, bridge) in &bridges {
            let frame = render::draw(
                screen.panel,
                &render::State {
                    usage: &usage,
                    cycle,
                },
            )?;
            bridge.show(&frame).map_err(describe)?;
        }
        return Ok(());
    }

    // A second set of bridges to the same device would interleave its frames
    // with the first one.
    daemon::take_lock()?;
    daemon::catch_signals();

    println!(
        "{}",
        tf!(
            "Panels on device {0}, updated every {1}: {2}",
            device.ip(),
            "5s",
            listing(&screens)
        )
    );

    let outcome = std::thread::scope(|scope| {
        let running: Vec<_> = bridges
            .iter()
            .map(|(screen, bridge)| scope.spawn(move || keep(screen, bridge)))
            .collect();

        // A screen that gave up brings the whole bridge down: the device is
        // gone, and the other screens are talking to the same one. The status
        // line starts everything again once it is back.
        running
            .into_iter()
            .filter_map(|thread| thread.join().ok())
            .find(Result::is_err)
            .unwrap_or(Ok(()))
    });

    daemon::drop_lock();
    outcome
}

/// One screen, until it is told to stop or the device stops answering.
fn keep(screen: &Screen, bridge: &Bridge) -> Outcome {
    let mut last = String::new();

    bridge
        .run(
            &Pacing {
                poll: POLL,
                give_up_after: GIVE_UP_AFTER,
                ..Default::default()
            },
            || {
                // Someone else took over, or the panels were turned off: leave
                // the screen to whoever owns it now.
                if daemon::stopping() || !daemon::owns_lock() {
                    return Ok(Tick::Stop);
                }

                let (usage, cycle) = state();
                let key = format!("{}|{}", usage.usage_key(), cycle_key(cycle));
                let frame = render::draw(
                    screen.panel,
                    &render::State {
                        usage: &usage,
                        cycle,
                    },
                )
                .map_err(Error::Malformed)?;

                // Growing percentages are why the panel hangs on the wall at
                // all; a countdown that moved by a minute can wait.
                let urgent = std::mem::replace(&mut last, key.clone()) != key;
                Ok(if urgent {
                    Tick::Now(frame)
                } else {
                    Tick::WhenIdle(frame)
                })
            },
            |err| eprintln!("{}", describe_ref(err)),
        )
        .map_err(|_| t("the device has been unreachable for too long, leaving").to_string())
}

/// The day count is what the renewal panel is about — the fraction of the bar
/// moves with it and needs no separate news.
fn cycle_key(cycle: Option<billing::Cycle>) -> String {
    cycle.map_or_else(String::new, |cycle| cycle.days_left.to_string())
}

fn build(config: &Config, device: &Device, screen: &Screen) -> Result<Bridge, String> {
    let mut builder = Bridge::builder(device.clone())
        .screen(screen.index)
        .port(config.port_of(screen.index));
    if screen.prev_clock_id != 0 {
        builder = builder.restore_to(ClockFace {
            id: screen.prev_clock_id,
            independence: screen.prev_independence,
        });
    }
    builder.build().map_err(describe)
}

/// Remembers the clock faces that were on the screens before we took them: come
/// uninstall time there would be nowhere else to get them from.
fn remember_clock_faces(bridges: &[(Screen, Bridge)]) -> Result<(), String> {
    let learned: Vec<(u8, ClockFace)> = bridges
        .iter()
        .filter(|(screen, _)| screen.prev_clock_id == 0)
        .filter_map(|(screen, bridge)| bridge.restores_to().map(|face| (screen.index, face)))
        .collect();
    if learned.is_empty() {
        return Ok(());
    }

    Config::update(move |saved| {
        for (index, face) in learned {
            if let Some(screen) = saved
                .screens
                .iter_mut()
                .find(|screen| screen.index == index)
            {
                screen.prev_clock_id = face.id;
                screen.prev_independence = face.independence;
            }
        }
    })
}

fn listing(screens: &[Screen]) -> String {
    screens
        .iter()
        .map(|screen| format!("{} — {}", screen.index + 1, screen.panel.title()))
        .collect::<Vec<_>>()
        .join(", ")
}

/// Finds the chosen device again and writes its current address into the
/// config. The known address is tried first — a sweep of the network on every
/// start would be paid for on every run, while the address usually holds.
fn locate(config: &mut Config) -> Result<Device, String> {
    if let Ok(ip) = config.ip.parse() {
        let device = Device::at(ip);
        if device
            .command(serde_json::json!({"Command": "Channel/GetAllConf"}))
            .is_ok()
        {
            return Ok(device);
        }
    }

    let found = discover(Discover::new()).map_err(describe)?;
    // Nobody is at the keyboard here, so a device that is gone is replaced only
    // when the network has exactly one Divoom to offer.
    let picked = pick_known(&found, config)
        .or_else(|| found.first().filter(|_| found.len() == 1).cloned())
        .ok_or_else(|| {
            t("the chosen device is not on the network — choose again: claudestatus setup")
                .to_string()
        })?;

    remember(config, &picked)?;
    Ok(picked)
}

/// The id and the MAC outlive a change of address, so they are asked first; a
/// config from an older version has neither and only knows the address.
pub fn pick_known(found: &[Device], config: &Config) -> Option<Device> {
    found
        .iter()
        .find(|device| config.device_id != 0 && device.id() == config.device_id)
        .or_else(|| {
            found
                .iter()
                .find(|device| !config.mac.is_empty() && device.mac() == config.mac)
        })
        .or_else(|| {
            (config.device_id == 0 && config.mac.is_empty())
                .then(|| {
                    found
                        .iter()
                        .find(|device| device.ip().to_string() == config.ip)
                })
                .flatten()
        })
        .cloned()
}

/// Only what the search really learned is written down: what the cloud once
/// told us must not be wiped by a sweep that went without it.
fn remember(config: &mut Config, device: &Device) -> Result<(), String> {
    config.ip = device.ip().to_string();
    if device.id() != 0 {
        config.device_id = device.id();
    }
    if !device.mac().is_empty() {
        config.mac = device.mac().to_string();
    }
    if !device.name().is_empty() {
        config.name = device.name().to_string();
    }

    let (ip, id, mac, name) = (
        config.ip.clone(),
        config.device_id,
        config.mac.clone(),
        config.name.clone(),
    );
    // Only the address is ours to write: the screens and the on/off flag may
    // have been changed by the human while we were looking.
    Config::update(move |saved| {
        saved.ip = ip;
        saved.device_id = id;
        saved.mac = mac;
        saved.name = name;
    })
}

/// Turns the panels back on with the device and the screens already chosen.
/// Choosing them is what `claudestatus setup` is for.
fn turn_on() -> Outcome {
    let mut config = match Config::load() {
        Ok(config) => config,
        Err(Missing::NeverTurnedOn) => {
            return Err(t("nothing is set up yet — claudestatus setup").into());
        }
        Err(broken) => return Err(broken.to_string()),
    };
    if config.ip.is_empty() {
        return Err(t("no device was ever chosen — claudestatus setup").into());
    }

    config.on = Some(true);
    config.save()?;

    daemon::ensure_running();
    // The bridge writes its pid file a moment after the fork, so a check right
    // away would report failure on a bridge that is in fact starting.
    for _ in 0..30 {
        if daemon::running() {
            println!(
                "{}",
                tf!("The panels are on: {0}", listing(&config.live_screens()))
            );
            return Ok(());
        }
        std::thread::sleep(Duration::from_millis(100));
    }
    Ok(())
}

fn turn_off() -> Outcome {
    if daemon::running() {
        daemon::stop();
    } else {
        daemon::restore();
    }

    // The config stays: the device and the screens chosen by the human are the
    // settings of the panels, not the state of a running bridge.
    match Config::load() {
        Err(Missing::NeverTurnedOn) => return Ok(()),
        Err(broken) => return Err(broken.to_string()),
        Ok(mut config) => {
            config.on = Some(false);
            config.save()?;
        }
    }

    // A bridge left over from an update or a second copy notices the cleared
    // flag only on its next tick, and until then it keeps drawing. Give it that
    // moment and put the clock faces back once more.
    std::thread::sleep(POLL + Duration::from_secs(1));
    daemon::restore();

    println!(
        "{}",
        t("The panels are off, the screens got their clock faces back")
    );
    Ok(())
}

pub fn label(device: &Device) -> String {
    if device.name().is_empty() {
        return t("Divoom device").into();
    }
    device.name().to_string()
}

/// Writes down the device the wizard picked.
pub fn adopt(config: &mut Config, device: &Device) {
    config.ip = device.ip().to_string();
    config.device_id = device.id();
    config.mac = device.mac().to_string();
    config.name = device.name().to_string();
}

/// The crate speaks English and knows nothing of catalogs; the wording the user
/// reads is ours to pick.
pub fn describe(err: Error) -> String {
    describe_ref(&err)
}

fn describe_ref(err: &Error) -> String {
    match err {
        Error::Unreachable(what) => tf!("the device is unreachable: {0}", what),
        Error::Refused(code) => tf!("the device rejected the command: {0}", code),
        Error::Malformed(what) => tf!("the device answered with something unexpected: {0}", what),
        Error::NotFound => t("no Divoom devices are visible on this network").into(),
        Error::FrameNotFetched => t("the device did not take the frame").into(),
        Error::NoSuchScreen(index) => tf!("the device has no screen {0}", index),
        Error::Io(err) => err.to_string(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn names_every_screen_it_holds() {
        let screens = vec![
            Screen::new(0, Panel::Limits),
            Screen::new(3, Panel::Renewal),
        ];
        let listing = listing(&screens);

        assert!(listing.contains('1'), "{listing}");
        assert!(
            listing.contains('4'),
            "the screens are numbered as the app does"
        );
    }

    #[test]
    fn tells_the_panels_apart_by_the_day_they_count() {
        let of = |now| cycle_key(Some(billing::cycle(20, now)));
        assert_eq!(of(1_786_000_000.0), of(1_786_000_000.0 + 600.0));
        assert_ne!(of(1_786_000_000.0), of(1_786_000_000.0 + 86_400.0 * 2.0));
        assert_eq!(cycle_key(None), "");
    }
}
