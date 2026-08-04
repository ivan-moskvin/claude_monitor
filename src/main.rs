//! The Claude limits status line, and its own upkeep.
//!
//! With no arguments it behaves as a `statusLine` command for Claude Code: the
//! session JSON arrives on stdin, the status line goes to stdout. Subcommands
//! exist for the human only: register in the settings, check for an update and
//! install it.

mod i18n;
mod install;
mod panel;
mod paths;
mod snapshot;
mod statusline;
mod update;

use i18n::tf;

/// What a subcommand answers with. An error is a message ready to be printed:
/// it is the CLI that speaks to the human, and it speaks the language i18n
/// picked.
pub type Outcome = Result<(), String>;

const USAGE: &str = "claudestatus — Claude limits in the Claude Code status line.

Usage:
  claudestatus            status line: session JSON on stdin, line on stdout
  claudestatus install    register in ~/.claude/settings.json
  claudestatus check      check whether a new version is out
  claudestatus update     download the latest version and replace itself
  claudestatus uninstall  remove the status line, the cache and the binary
  claudestatus divoom     limits panel on a Divoom Times Gate (divoom help)
  claudestatus version    print the version
  claudestatus help       this help

Environment:
  CLAUDESTATUS_LANG=ru|en         force the interface language
  CLAUDESTATUS_NO_AUTO_UPDATE=1   do not check for updates in the background
";

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let Some(command) = args.first() else {
        // The check is started in any case: even empty input means Claude Code
        // is alive and somebody is looking at the line.
        statusline::run(update::available());
        // The panel lives only while the bridge is running. Start it from here:
        // the check takes milliseconds and does nothing without a Times Gate on
        // the network.
        panel::ensure_running();
        update::auto_check();
        return;
    };

    // --install is still understood: old install scripts spell it that way.
    let result: Outcome = match command.as_str() {
        "install" | "--install" => install::install(),
        "uninstall" => install::uninstall(),
        "divoom" => panel::run(&args[1..]),
        "check" => update::check(args.iter().any(|arg| arg == "--quiet")),
        "update" => update::update(),
        "version" | "--version" | "-v" => {
            println!("{}", version());
            Ok(())
        }
        "help" | "--help" | "-h" => {
            print!("{}", i18n::t(USAGE));
            Ok(())
        }
        other => {
            eprint!(
                "{}",
                tf!("Unknown command: {0}\n\n{1}", other, i18n::t(USAGE))
            );
            std::process::exit(2);
        }
    };

    if let Err(err) = result {
        eprintln!("{err}");
        std::process::exit(1);
    }
}

/// The version is stamped into a release build by CI. A hand-made build has
/// none — such a build counts as outdated and never lights the update mark up.
pub fn version() -> String {
    match option_env!("CLAUDESTATUS_VERSION") {
        Some(tag) if !tag.is_empty() => tag.to_string(),
        _ => i18n::t("built from source").to_string(),
    }
}
