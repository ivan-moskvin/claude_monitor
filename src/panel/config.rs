//! The settings of the panel: which device, which screens, what goes on each of
//! them.

use serde::{Deserialize, Serialize};

use crate::i18n::{t, tf};
use crate::paths;

const NAME: &str = "divoom.json";

/// The default port of the local server with the frames. A screen takes this
/// port plus its own index: every screen is served by a bridge of its own, and
/// the ports have to stay fixed across restarts — the device may still hold a
/// link from the previous run.
const DEFAULT_PORT: u16 = 8477;

/// The screen the limits take unless told otherwise — the rightmost one.
const DEFAULT_SCREEN: u8 = 4;

/// What a screen shows.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Panel {
    /// The three bars: the five-hour window, the time to its reset, the week.
    Limits,
    /// The five-hour window alone, large enough to read across a room.
    FiveHour,
    /// The weekly window alone.
    Week,
    /// The days until the subscription renews.
    Renewal,
}

impl Panel {
    /// Every panel there is, in the order the wizard offers them.
    pub const ALL: &'static [Panel] =
        &[Panel::Limits, Panel::FiveHour, Panel::Week, Panel::Renewal];

    /// The name the human reads. Localized at the call site, not stored.
    pub fn title(self) -> &'static str {
        match self {
            Panel::Limits => t("limits: 5h, reset, week"),
            Panel::FiveHour => t("the five-hour window"),
            Panel::Week => t("the weekly window"),
            Panel::Renewal => t("days until the subscription renews"),
        }
    }

    /// Whether the panel is worth drawing at all with the settings we have. A
    /// renewal panel without a billing day would show a blank.
    pub fn ready(self, config: &Config) -> bool {
        match self {
            Panel::Renewal => config.billing_day.is_some(),
            _ => true,
        }
    }
}

/// A screen of the device and what it was showing before we took it.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Screen {
    /// Screen 0–4. The Divoom app labels them 1 to 5.
    pub index: u8,
    pub panel: Panel,
    /// What was on this screen before us. Remembered when the bridge first
    /// takes it and given back on uninstall — otherwise the screen is left with
    /// a dead picture once the bridge is gone.
    #[serde(default, skip_serializing_if = "is_zero")]
    pub prev_clock_id: u32,
    #[serde(default, skip_serializing_if = "is_zero")]
    pub prev_independence: u32,
}

impl Screen {
    pub fn new(index: u8, panel: Panel) -> Self {
        Self {
            index,
            panel,
            prev_clock_id: 0,
            prev_independence: 0,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    /// Whether the panel is turned on. `off` clears the flag but keeps the
    /// file: the chosen device and screens have to survive it, or every `on`
    /// would start over from the defaults. A config written before the flag
    /// existed carries no value and counts as on.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub on: Option<bool>,
    /// The last known address of the device. DHCP moves it, so it is a hint for
    /// the search, never the answer.
    #[serde(default)]
    pub ip: String,
    /// The base port; screen N is served on `port + N`.
    #[serde(default = "default_port")]
    pub port: u16,
    /// The id of the device in the Divoom cloud — the screen layout is looked
    /// up by it, and it tells our device from the neighbouring ones after a
    /// move.
    #[serde(default, skip_serializing_if = "is_zero")]
    pub device_id: u32,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub mac: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub name: String,
    /// The screens we hold, and what each of them shows. The ones not listed
    /// are left to whoever owns them.
    #[serde(default)]
    pub screens: Vec<Screen>,
    /// The day of the month the subscription is charged on. Nothing tells us
    /// this — Claude Code reports the rolling windows and never the billing
    /// cycle — so it is asked once and kept here.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub billing_day: Option<u8>,

    /// What a config from before the screens said. Read so that an existing
    /// setup keeps its screen, never written back: `screens` is where it lives
    /// now.
    #[serde(default, skip_serializing)]
    lcd_index: Option<u8>,
    #[serde(default, skip_serializing)]
    prev_clock_id: Option<u32>,
    #[serde(default, skip_serializing)]
    prev_independence: Option<u32>,
}

fn default_port() -> u16 {
    DEFAULT_PORT
}

fn is_zero(value: &u32) -> bool {
    *value == 0
}

impl Default for Config {
    fn default() -> Self {
        Self {
            on: None,
            ip: String::new(),
            port: DEFAULT_PORT,
            device_id: 0,
            mac: String::new(),
            name: String::new(),
            screens: vec![Screen::new(DEFAULT_SCREEN, Panel::Limits)],
            billing_day: None,
            lcd_index: None,
            prev_clock_id: None,
            prev_independence: None,
        }
    }
}

/// Why there is no config to work with. A file that was never written is told
/// apart from a broken one: the second has to be reported rather than quietly
/// replaced by the defaults.
#[derive(Debug)]
pub enum Missing {
    NeverTurnedOn,
    Broken(String),
}

impl std::fmt::Display for Missing {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Missing::NeverTurnedOn => write!(
                f,
                "{}",
                t("the panel is not turned on — claudestatus setup")
            ),
            Missing::Broken(what) => write!(f, "{what}"),
        }
    }
}

impl Config {
    /// A setup that does not exist yet. Unlike the default it holds no screen:
    /// the wizard asks about every one of them, and an assumed panel would be
    /// one nobody picked.
    pub fn fresh() -> Config {
        Config {
            screens: Vec::new(),
            ..Default::default()
        }
    }

    pub fn load() -> Result<Config, Missing> {
        let path = paths::file(NAME).map_err(Missing::Broken)?;

        let data = match std::fs::read(&path) {
            Ok(data) => data,
            Err(err) if err.kind() == std::io::ErrorKind::NotFound => {
                return Err(Missing::NeverTurnedOn);
            }
            Err(err) => return Err(Missing::Broken(err.to_string())),
        };

        let mut config: Config = serde_json::from_slice(&data)
            .map_err(|err| Missing::Broken(tf!("{0} does not parse: {1}", path.display(), err)))?;
        config.adopt_legacy();
        if config.port == 0 {
            config.port = DEFAULT_PORT;
        }
        Ok(config)
    }

    /// A config from before the screens knew one screen and one panel. It is
    /// taken over as it was: an update must not move the panel out from under
    /// somebody, nor lose the clock face that screen has to be given back.
    fn adopt_legacy(&mut self) {
        if !self.screens.is_empty() {
            return;
        }
        let Some(index) = self.lcd_index else { return };
        self.screens = vec![Screen {
            index,
            panel: Panel::Limits,
            prev_clock_id: self.prev_clock_id.unwrap_or(0),
            prev_independence: self.prev_independence.unwrap_or(0),
        }];
    }

    pub fn save(&self) -> Result<(), String> {
        let path = paths::file(NAME)?;
        let mut data = serde_json::to_vec_pretty(self).map_err(|err| err.to_string())?;
        data.push(b'\n');
        std::fs::write(&path, data)
            .map_err(|err| tf!("could not write {0}: {1}", path.display(), err))
    }

    /// Re-reads the file and lets `change` touch only the fields it means to.
    ///
    /// The bridge lives for hours with a copy of the config in memory, while
    /// the human keeps giving orders — `setup`, `off` — through another
    /// process. Saving the whole copy would put a stale answer back over a
    /// fresh one; the last word belongs to whoever spoke last.
    pub fn update(change: impl FnOnce(&mut Config)) -> Result<(), String> {
        let mut config = Config::load().map_err(|err| err.to_string())?;
        change(&mut config);
        config.save()
    }

    pub fn enabled(&self) -> bool {
        self.on.unwrap_or(true)
    }

    /// The screens worth running a bridge for: a panel that has nothing to draw
    /// would take a screen only to show a blank.
    pub fn live_screens(&self) -> Vec<Screen> {
        self.screens
            .iter()
            .filter(|screen| screen.panel.ready(self))
            .cloned()
            .collect()
    }

    pub fn screen(&self, index: u8) -> Option<&Screen> {
        self.screens.iter().find(|screen| screen.index == index)
    }

    /// Gives a screen a panel, or takes it away with `None`. The clock face of
    /// a screen we already hold is kept — it is the only record of what has to
    /// be given back.
    pub fn set_screen(&mut self, index: u8, panel: Option<Panel>) {
        match panel {
            None => self.screens.retain(|screen| screen.index != index),
            Some(panel) => match self.screens.iter_mut().find(|screen| screen.index == index) {
                Some(screen) => screen.panel = panel,
                None => self.screens.push(Screen::new(index, panel)),
            },
        }
        self.screens.sort_by_key(|screen| screen.index);
    }

    /// The port the frames for a screen are served on.
    pub fn port_of(&self, index: u8) -> u16 {
        self.port.saturating_add(index as u16)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn parse(json: &str) -> Config {
        let mut config: Config = serde_json::from_str(json).unwrap();
        config.adopt_legacy();
        config
    }

    #[test]
    fn counts_a_config_from_before_the_flag_as_on() {
        assert!(parse(r#"{"ip":"192.168.1.5","lcd_index":1,"port":8477}"#).enabled());
    }

    #[test]
    fn remembers_being_turned_off() {
        assert!(!parse(r#"{"ip":"192.168.1.5","on":false}"#).enabled());
    }

    #[test]
    fn takes_over_a_config_from_before_the_screens() {
        let config = parse(
            r#"{"ip":"192.168.1.5","lcd_index":1,"prev_clock_id":104,"prev_independence":745289}"#,
        );

        assert_eq!(config.screens.len(), 1);
        let screen = &config.screens[0];
        assert_eq!(screen.index, 1, "the panel stays where it was");
        assert_eq!(screen.panel, Panel::Limits);
        assert_eq!(
            screen.prev_clock_id, 104,
            "the clock face to give back is not lost"
        );
        assert_eq!(screen.prev_independence, 745289);
    }

    #[test]
    fn writes_the_screens_the_new_way_only() {
        let config = parse(r#"{"ip":"192.168.1.5","lcd_index":1,"prev_clock_id":104}"#);
        let written = serde_json::to_string(&config).unwrap();

        assert!(written.contains("\"screens\""), "{written}");
        assert!(!written.contains("lcd_index"), "{written}");
    }

    #[test]
    fn leaves_the_screens_alone_when_the_config_already_has_them() {
        let config = parse(
            r#"{"ip":"192.168.1.5","lcd_index":0,"screens":[{"index":3,"panel":"renewal"}]}"#,
        );

        assert_eq!(config.screens.len(), 1);
        assert_eq!(config.screens[0].index, 3);
        assert_eq!(config.screens[0].panel, Panel::Renewal);
    }

    #[test]
    fn fills_in_what_an_older_config_does_not_say() {
        assert_eq!(parse(r#"{"ip":"192.168.1.5"}"#).port, DEFAULT_PORT);
    }

    #[test]
    fn writes_nothing_it_does_not_know() {
        let written = serde_json::to_string(&Config {
            ip: "192.168.1.5".into(),
            screens: Vec::new(),
            ..Default::default()
        })
        .unwrap();

        assert!(!written.contains("device_id"), "{written}");
        assert!(!written.contains("prev_clock_id"), "{written}");
        assert!(!written.contains("billing_day"), "{written}");
        assert!(!written.contains("\"on\""), "{written}");
    }

    #[test]
    fn keeps_what_it_was_told_across_a_write_and_a_read() {
        let config = Config {
            on: Some(true),
            ip: "192.168.1.5".into(),
            port: 4321,
            device_id: 300373815,
            mac: "aa:bb".into(),
            name: "Times Gate".into(),
            screens: vec![
                Screen::new(0, Panel::Limits),
                Screen {
                    index: 4,
                    panel: Panel::Renewal,
                    prev_clock_id: 104,
                    prev_independence: 745289,
                },
            ],
            billing_day: Some(20),
            ..Default::default()
        };
        let read = parse(&serde_json::to_string(&config).unwrap());

        assert_eq!(read.device_id, 300373815);
        assert_eq!(read.billing_day, Some(20));
        assert_eq!(read.screens.len(), 2);
        assert_eq!(read.screens[1].panel, Panel::Renewal);
        assert_eq!(
            read.screens[1].prev_independence, 745289,
            "the grouping of the screens is not a flag"
        );
    }

    #[test]
    fn gives_a_screen_a_panel_and_takes_it_away() {
        let mut config = Config {
            screens: Vec::new(),
            ..Default::default()
        };

        config.set_screen(3, Some(Panel::Renewal));
        config.set_screen(0, Some(Panel::Limits));
        assert_eq!(
            config.screens.iter().map(|s| s.index).collect::<Vec<_>>(),
            vec![0, 3],
            "kept in the order of the device"
        );

        config.set_screen(3, Some(Panel::Week));
        assert_eq!(
            config.screens.len(),
            2,
            "the same screen is not added twice"
        );
        assert_eq!(config.screen(3).unwrap().panel, Panel::Week);

        config.set_screen(3, None);
        assert!(config.screen(3).is_none());
    }

    #[test]
    fn keeps_the_clock_face_when_the_panel_of_a_screen_changes() {
        let mut config = Config {
            screens: vec![Screen {
                index: 2,
                panel: Panel::Limits,
                prev_clock_id: 104,
                prev_independence: 745289,
            }],
            ..Default::default()
        };

        config.set_screen(2, Some(Panel::Renewal));

        let screen = config.screen(2).unwrap();
        assert_eq!(
            screen.prev_clock_id, 104,
            "the screen is still ours and still has to be given back"
        );
    }

    #[test]
    fn serves_every_screen_on_a_port_of_its_own() {
        let config = Config::default();
        assert_eq!(config.port_of(0), DEFAULT_PORT);
        assert_eq!(config.port_of(4), DEFAULT_PORT + 4);
    }

    #[test]
    fn does_not_run_a_panel_that_has_nothing_to_draw() {
        let mut config = Config {
            screens: vec![
                Screen::new(0, Panel::Limits),
                Screen::new(1, Panel::Renewal),
            ],
            billing_day: None,
            ..Default::default()
        };
        assert_eq!(
            config.live_screens().len(),
            1,
            "the renewal panel has no billing day yet"
        );

        config.billing_day = Some(20);
        assert_eq!(config.live_screens().len(), 2);
    }
}
