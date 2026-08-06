//! Choosing from a list with the arrow keys.
//!
//! Typing a number and pressing Enter is what a wizard does when it has no
//! terminal to speak of; this one has. The terminal is put into raw mode for as
//! long as a question is on the screen and given back afterwards — including
//! when the answer is a Ctrl-C, or the shell would be left without an echo.
//!
//! Nothing here knows what is being chosen: the caller hands over the lines and
//! gets back the index of the one that was picked.

use std::io::{Read, Write};

use crate::i18n::t;

/// What a keypress meant. Everything else is ignored — a wizard that answers a
/// stray key with a complaint is worse than one that says nothing.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Key {
    Up,
    Down,
    /// Enter or the space bar: both confirm, because both are what a hand
    /// reaches for.
    Confirm,
    Cancel,
}

/// Asks which of `items` it is to be, starting the cursor at `current`.
/// Answers `None` when the question was cancelled.
pub fn select(title: &str, items: &[String], current: usize) -> Result<Option<usize>, String> {
    if items.is_empty() {
        return Ok(None);
    }
    let mut at = current.min(items.len() - 1);

    let Some(raw) = RawMode::enter() else {
        // A terminal that will not go raw still has to be usable: the numbered
        // list is the fallback, not the interface.
        return numbered(title, items, at);
    };

    println!("{title}");
    println!("  {}", t("↑↓ choose   Enter confirm"));
    draw(items, at, false);

    let mut keys = Keys::default();
    let picked = loop {
        match keys.next() {
            Key::Up => at = (at + items.len() - 1) % items.len(),
            Key::Down => at = (at + 1) % items.len(),
            Key::Confirm => break Some(at),
            Key::Cancel => break None,
        }
        // Back up over the list and write it again: the lines are the same
        // height every time, so nothing scrolls and nothing flickers.
        print!("\x1b[{}A", items.len());
        draw(items, at, false);
    };

    // The chosen line is left on the screen without the cursor mark, so that the
    // answers read as a transcript once the wizard has moved on.
    print!("\x1b[{}A", items.len());
    draw(items, picked.unwrap_or(at), true);
    drop(raw);

    Ok(picked)
}

/// Yes or no, asked the same way as everything else: no letter to press means
/// no keyboard layout to switch.
pub fn confirm(question: &str, default_yes: bool) -> Result<bool, String> {
    let items = vec![t("yes").to_string(), t("no").to_string()];
    let at = if default_yes { 0 } else { 1 };
    Ok(select(question, &items, at)?
        .map(|picked| picked == 0)
        .unwrap_or(false))
}

fn draw(items: &[String], at: usize, settled: bool) {
    for (index, item) in items.iter().enumerate() {
        let chosen = index == at;
        // \x1b[K clears what the previous, possibly longer line left behind.
        if chosen && !settled {
            println!("\x1b[K  \x1b[1m› {item}\x1b[0m");
        } else if chosen {
            println!("\x1b[K  \x1b[1m  {item}\x1b[0m");
        } else if settled {
            println!("\x1b[K");
        } else {
            println!("\x1b[K    \x1b[2m{item}\x1b[0m");
        }
    }
    if settled {
        // The blanked lines are still below the cursor; step back over them so
        // the next question starts where the answer is.
        print!("\x1b[{}A", items.len() - 1);
        let _ = std::io::stdout().flush();
    }
}

/// The keys still unread. A single read brings back as much as the terminal has
/// to offer — an escape sequence, but also a whole handful of keypresses when
/// they were pasted or piped in — so what arrives is parsed key by key and
/// nothing is thrown away.
#[derive(Default)]
struct Keys(Vec<u8>);

impl Keys {
    fn next(&mut self) -> Key {
        loop {
            match key_of(&self.0) {
                Taken::Key(key, used) => {
                    self.0.drain(..used);
                    return key;
                }
                Taken::Ignore(used) => {
                    self.0.drain(..used);
                }
                // Nothing there yet, or half an escape sequence: read more.
                Taken::More => {
                    let mut buffer = [0u8; 32];
                    match std::io::stdin().read(&mut buffer) {
                        // The input ended — a closed terminal, or answers that
                        // ran out. Anything but leaving would spin forever.
                        Ok(0) | Err(_) => return Key::Cancel,
                        Ok(read) => self.0.extend_from_slice(&buffer[..read]),
                    }
                }
            }
        }
    }
}

/// What the bytes at the front of the queue turned out to be, and how many of
/// them the answer took.
#[derive(Debug, PartialEq, Eq)]
enum Taken {
    Key(Key, usize),
    Ignore(usize),
    More,
}

/// Kept apart from the reading so that it can be checked without a terminal.
fn key_of(bytes: &[u8]) -> Taken {
    match bytes {
        [] => Taken::More,
        [b'\r' | b'\n' | b' ', ..] => Taken::Key(Key::Confirm, 1),
        // Ctrl-C and Ctrl-D: a wizard has to be leavable the usual way.
        [3 | 4, ..] | [b'q', ..] => Taken::Key(Key::Cancel, 1),
        [b'k', ..] => Taken::Key(Key::Up, 1),
        [b'j', ..] => Taken::Key(Key::Down, 1),
        [0x1b, b'[' | b'O', b'A', ..] => Taken::Key(Key::Up, 3),
        [0x1b, b'[' | b'O', b'B', ..] => Taken::Key(Key::Down, 3),
        // An arrow that is neither up nor down, and every other three-byte
        // sequence: dropped whole, or its tail would read as letters.
        [0x1b, b'[' | b'O', _, ..] => Taken::Ignore(3),
        // A lone Escape is a cancel — but only once we know nothing follows it.
        [0x1b] | [0x1b, b'[' | b'O'] => Taken::More,
        [0x1b, ..] => Taken::Key(Key::Cancel, 1),
        [_, ..] => Taken::Ignore(1),
    }
}

/// The fallback for a terminal that will not go raw.
fn numbered(title: &str, items: &[String], current: usize) -> Result<Option<usize>, String> {
    println!("{title}");
    for (index, item) in items.iter().enumerate() {
        let mark = if index == current { "›" } else { " " };
        println!("  {mark} {}. {item}", index + 1);
    }
    print!("{}", t("number, or Enter to keep: "));
    let _ = std::io::stdout().flush();

    let mut answer = String::new();
    std::io::stdin()
        .read_line(&mut answer)
        .map_err(|err| err.to_string())?;
    let answer = answer.trim();

    if answer.is_empty() {
        return Ok(Some(current));
    }
    match answer.parse::<usize>() {
        Ok(number) if number >= 1 && number <= items.len() => Ok(Some(number - 1)),
        _ => numbered(title, items, current),
    }
}

/// The terminal as it was before the question, put back when this is dropped.
struct RawMode(Saved);

impl RawMode {
    fn enter() -> Option<Self> {
        let saved = raw_on()?;
        // The cursor would otherwise sit on the list and blink at a line it has
        // nothing to do with.
        print!("\x1b[?25l");
        let _ = std::io::stdout().flush();
        Some(RawMode(saved))
    }
}

impl Drop for RawMode {
    fn drop(&mut self) {
        print!("\x1b[?25h");
        let _ = std::io::stdout().flush();
        raw_off(&self.0);
    }
}

#[cfg(unix)]
type Saved = libc::termios;

#[cfg(unix)]
fn raw_on() -> Option<Saved> {
    unsafe {
        let mut saved: libc::termios = std::mem::zeroed();
        if libc::tcgetattr(libc::STDIN_FILENO, &mut saved) != 0 {
            return None;
        }
        let mut raw = saved;
        // Keys have to arrive as they are pressed and without being echoed.
        // ISIG goes too, so that Ctrl-C arrives as a byte we can leave on:
        // killed by the signal instead, we would never put the terminal back
        // and the shell would be left without an echo.
        raw.c_lflag &= !(libc::ICANON | libc::ECHO | libc::ISIG);
        raw.c_cc[libc::VMIN] = 1;
        raw.c_cc[libc::VTIME] = 0;
        if libc::tcsetattr(libc::STDIN_FILENO, libc::TCSANOW, &raw) != 0 {
            return None;
        }
        Some(saved)
    }
}

#[cfg(unix)]
fn raw_off(saved: &Saved) {
    unsafe {
        libc::tcsetattr(libc::STDIN_FILENO, libc::TCSANOW, saved);
    }
}

#[cfg(windows)]
type Saved = u32;

/// Windows spells the arrows its own way unless the console is asked for the
/// virtual terminal input every other terminal already speaks.
#[cfg(windows)]
fn raw_on() -> Option<Saved> {
    use windows_sys::Win32::System::Console::{
        ENABLE_ECHO_INPUT, ENABLE_LINE_INPUT, ENABLE_VIRTUAL_TERMINAL_INPUT, GetConsoleMode,
        GetStdHandle, STD_INPUT_HANDLE, SetConsoleMode,
    };

    unsafe {
        let handle = GetStdHandle(STD_INPUT_HANDLE);
        if handle.is_null() {
            return None;
        }
        let mut saved = 0u32;
        if GetConsoleMode(handle, &mut saved) == 0 {
            return None;
        }
        let raw =
            (saved & !(ENABLE_LINE_INPUT | ENABLE_ECHO_INPUT)) | ENABLE_VIRTUAL_TERMINAL_INPUT;
        if SetConsoleMode(handle, raw) == 0 {
            return None;
        }
        Some(saved)
    }
}

#[cfg(windows)]
fn raw_off(saved: &Saved) {
    use windows_sys::Win32::System::Console::{GetStdHandle, STD_INPUT_HANDLE, SetConsoleMode};

    unsafe {
        let handle = GetStdHandle(STD_INPUT_HANDLE);
        if !handle.is_null() {
            SetConsoleMode(handle, *saved);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Every key the bytes hold, the way the queue would hand them over.
    fn keys_of(bytes: &[u8]) -> Vec<Key> {
        let mut queue = Keys(bytes.to_vec());
        let mut found = Vec::new();
        // The queue reads from stdin once it runs dry; stop before that.
        while !queue.0.is_empty() {
            let before = queue.0.len();
            match key_of(&queue.0) {
                Taken::More => break,
                Taken::Key(key, used) => {
                    queue.0.drain(..used);
                    found.push(key);
                }
                Taken::Ignore(used) => {
                    queue.0.drain(..used);
                }
            }
            assert!(queue.0.len() < before, "the queue is not moving");
        }
        found
    }

    #[test]
    fn reads_the_arrows_every_terminal_sends() {
        assert_eq!(keys_of(&[0x1b, b'[', b'A']), vec![Key::Up]);
        assert_eq!(keys_of(&[0x1b, b'[', b'B']), vec![Key::Down]);
        // The application cursor mode of some terminals.
        assert_eq!(keys_of(&[0x1b, b'O', b'A']), vec![Key::Up]);
        assert_eq!(keys_of(&[0x1b, b'O', b'B']), vec![Key::Down]);
    }

    #[test]
    fn takes_enter_and_the_space_bar_for_the_same_thing() {
        assert_eq!(keys_of(b"\r"), vec![Key::Confirm]);
        assert_eq!(keys_of(b"\n"), vec![Key::Confirm]);
        assert_eq!(keys_of(b" "), vec![Key::Confirm]);
    }

    #[test]
    fn can_be_left_the_way_a_terminal_program_is_left() {
        assert_eq!(keys_of(&[3]), vec![Key::Cancel], "Ctrl-C");
        assert_eq!(keys_of(&[4]), vec![Key::Cancel], "Ctrl-D");
        assert_eq!(keys_of(b"q"), vec![Key::Cancel]);
        // Escape is a cancel once something follows it: on its own it could
        // still turn out to be the head of an arrow.
        assert_eq!(keys_of(&[0x1b, b'x']), vec![Key::Cancel]);
    }

    #[test]
    fn keeps_every_key_of_a_handful_that_arrived_at_once() {
        // A pasted or piped answer comes in one read, and losing all but the
        // first key of it would hang the wizard on a question nobody can see.
        assert_eq!(
            keys_of(b"\x1b[B\r\x1b[A\r"),
            vec![Key::Down, Key::Confirm, Key::Up, Key::Confirm]
        );
    }

    #[test]
    fn ignores_a_key_it_has_no_meaning_for() {
        assert_eq!(keys_of(b"x"), vec![]);
        assert_eq!(keys_of(&[]), vec![]);
        // A key of its own that happens to be an arrow we do not use: dropped
        // whole, or the letter of it would read as a keypress.
        assert_eq!(keys_of(b"\x1b[C\r"), vec![Key::Confirm], "the right arrow");
    }

    #[test]
    fn waits_for_the_rest_of_a_sequence_it_has_only_the_head_of() {
        assert_eq!(key_of(&[0x1b]), Taken::More);
        assert_eq!(key_of(&[0x1b, b'[']), Taken::More);
    }

    #[test]
    fn the_letters_of_the_arrows_work_too() {
        // Whoever reaches for them knows what they are doing, and it costs a line.
        assert_eq!(keys_of(b"k"), vec![Key::Up]);
        assert_eq!(keys_of(b"j"), vec![Key::Down]);
    }
}
