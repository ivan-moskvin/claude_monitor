# claudestatus divoom

A panel with the Claude limits on the screen of a [Divoom Times Gate](https://divoom.com/products/time-gate).

It reads the same `usage-snapshot.json` the status line writes, draws a 128×128
panel and hands it to one screen of the device. The other four screens are left
alone.

It lives as a subcommand of the main CLI — there is no separate binary:

```bash
claudestatus divoom on               # find the device and turn the panel on
claudestatus divoom off              # turn it off and give the clock face back
claudestatus divoom                  # the bridge: updates the panel while running
claudestatus divoom once             # send one frame and exit
claudestatus divoom preview p.gif    # look at a frame without touching the device
```

The settings are `divoom.json` in the application directory (created by `on`, mode 600):

```json
{ "ip": "192.168.1.50", "lcd_index": 4, "port": 8477 }
```

The device is found on its own; `ip` may be left empty — the device is looked up
through the Divoom cloud directory by shared public address. That is the only
request going outside; with `ip` in the config the bridge works over the local
network only.

## The Times Gate protocol

Verified on firmware Hardware 400. The knowledge is anything but obvious and was
gathered empirically — without it commands "succeed" and nothing happens.

- **The documentation demands a `LocalToken` in every command, but the firmware
  does not check it.** `Device/PlayGif` and `Channel/SetClockSelectId` run for
  anybody on the local network — verified, the frame was downloaded and shown by
  a command with no token.
- **`error_code: 0` does not mean "drawn"** — only "the command was parsed".
- **`"Request data illegal json"` means "unknown command"**, not a JSON parsing
  error: a nonexistent `Totally/Bogus` gives the very same answer. So
  `Draw/SendHttpItemList` is absent from this firmware, although the TimeGate
  documentation describes it.
- **`"Request data illegal json"` also comes back for a command without a
  `LocalToken`** — that is, in the same words as for a nonexistent one. It makes
  it easy to decide the firmware has no such command: `Draw/SendHttpText` and
  `Draw/SendHttpItemList` answer exactly like that without a token, and work
  with one.
- **`Draw/SendHttpGif` draws nothing and ruins the neighbouring screen.** It
  accepts the frame (base64 JPEG, `LcdArray`, an increasing `PicID`), answers
  zero, shows nothing on the target screen — and puts the neighbouring one into
  endless loading, out of which only `Channel/SetClockSelectId` pulls it.
  Verified twice; do not use.
- **The channel that works is `Device/PlayGif`**: the device downloads the GIF by
  a link itself. That is why the bridge brings up a local HTTP server and sends
  it the address. The port is fixed: after the bridge restarts, the screen still
  holds a link from the previous run, and it has to keep working.
- **`Draw/SendHttpItemList` does draw as well** — a `BackgroudGif` background
  plus text items, and the background is downloaded even with `NewFlag: 1`,
  although the documentation promises otherwise. The bridge does not use it: our
  own graphics say more than a set of text fields, and it does not save us from
  the loading indicator anyway.

## About the loading indicator

The screen blinks the indicator on every update, and there is no way around it.
Verified: loading shows up even when a single text item is changed through
`SendHttpItemList` — without a single network request from the device. So it is
caused by the screen being redrawn by an external command, not by a file being
downloaded; shrinking the picture makes no difference.

Only the built-in widgets update without loading, because the firmware pulls
their data itself. The same mode is available from outside — the "DIY Net Data
Clock": a clock face is created by hand in the Divoom app, given an
`InputUrlAddress` (our JSON) and `DataParsingRules` of the form `n:FiveHour`,
after which the device polls the address by itself. A local address suits it —
an item of `type: 23` went to `192.168.1.10:8477` every `update_time` seconds
without fail. The price is that it draws the clock face template, with no layout
of our own, which is why this path was rejected.

So the bridge is thrifty with updates: percentages go out at once, a shift of
the countdown waits for `minSendInterval`, an unchanged frame is not sent at all.
- The hash of the frame in the file name is what makes the device re-download
  the picture: at the same address it shows the same image as before.
- Screens are addressed by `LcdArray: [0,0,0,0,1]` (the fifth one), not by
  `LcdIndex`.
- The device comes for the picture in a separate request, with a delay: a
  one-shot send has to wait for that request, or the server closes earlier.

Handy commands while debugging (`POST http://<ip>/post`):

| Command | What for |
|---|---|
| `Channel/GetAllConf` | brightness and `LightSwitch` — whether the screen is on |
| `Channel/OnOffScreen` + `OnOff` | turn a screen on; without the parameter it blanks all five |
| `Channel/SetClockSelectId` | give the screen its ordinary clock face back |
| `https://app.divoom-gz.com/Channel/Get5LcdInfoV2?DeviceType=LCD&DeviceId=<id>` | the current layout of all five screens, to restore it |

## The panel

Three bars, as in the status line: the five-hour window, the time until it
resets, the weekly window. The color thresholds are the same (base color up to
60%, orange up to 85%, red above) and live in `usageWindow.tint`.

Two states are shown separately, without which the panel would lie silently: a
snapshot older than 90 seconds (the age mark next to the header, `2H` — the
numbers only grow during an active Claude Code session) and a window that has
reset (`RESET` — the percentages describe the window that is over, usage starts
from zero).

The font is our own 5×7 bitmap in `font.go`; there is no font engine among the
dependencies. It carries the Cyrillic letters as well, because the labels are
localized — a character with no glyph would come out as `?`. The countdown is
written as `2:38` and not with letters: in this grid the Russian letter for an
hour is indistinguishable from a four.
