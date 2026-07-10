# webterminal

A browser-based terminal that runs **on** the Pi itself — a single static
binary with an embedded xterm.js interface. Built specifically to work on old
hardware: Raspberry Pi 1 / Zero (ARMv6), Revolution Pi (RevPi Core / Connect),
and anything newer.

- ~6.5 MB binary, ~10 MB RAM at idle, zero dependencies on the device
- Full terminal control: colors, resize, interactive programs (htop, nano, vim)
- **Sessions survive page refreshes and network drops**: the shell keeps
  running on the Pi and the browser re-attaches with recent output replayed
  (grace period configurable via `-grace`, default 5 minutes)
- Auto-reconnect, adjustable font size
- Copy & paste that works over plain HTTP, mobile selection mode,
  shortcut key bar (Esc, Tab, Ctrl, arrows…)
- HTTP Basic Auth (see [security notes](#security-notes--read-this))

Full dependency list, compatibility matrix, and troubleshooting:
**[DEPENDENCIES.md](DEPENDENCIES.md)**

---

## Install

### 1. Pick your binary

| Your device | OS | Download |
|---|---|---|
| Pi 1, Pi Zero/Zero W, **all Revolution Pi models**, Pi 2–5 | 32-bit (Raspberry Pi OS / Revolution Pi OS) | `webterminal-armv6` |
| Pi 3/4/400/5, Zero 2 W | 64-bit Raspberry Pi OS | `webterminal-arm64` |
| x86-64 Linux machine | any | `webterminal-amd64` |

Not sure? Run `getconf LONG_BIT` on the device: `32` → armv6, `64` → arm64.

### 2. Download and install (run on the Pi)

```sh
wget https://github.com/muratbilge/webterminal/releases/latest/download/webterminal-armv6
sudo install -m755 webterminal-armv6 /usr/local/bin/webterminal
```

(Substitute the binary name from the table above. To verify the download:
also fetch `SHA256SUMS.txt` and run `sha256sum -c SHA256SUMS.txt --ignore-missing`.)

### 3. First run

```sh
WT_USER=admin WT_PASS=YOUR_PASSWORD webterminal -addr :8080
```

(Credentials can also be given as `-user`/`-pass` flags, but flags are visible
to all local users in `ps` output — prefer the environment variables.)

Open `http://<pi-address>:8080` in any browser, log in — you have a shell.

### 4a. Run without a service

Nothing else is required — just run the command from step 3 whenever you need
it. To keep it running after you close your SSH session:

```sh
WT_USER=admin WT_PASS=YOUR_PASSWORD nohup webterminal -addr :8080 >/dev/null 2>&1 &
```

Stop it with `pkill webterminal`. To start it automatically at boot without
systemd, add that same line (using env vars for the credentials) to
`/etc/rc.local` before the `exit 0`, or as a cron entry: `@reboot`.

### 4b. Optional: install as a service (start on boot, auto-restart)

```sh
wget https://github.com/muratbilge/webterminal/releases/latest/download/webterminal.service
nano webterminal.service    # set User= (which account the shell runs as) and WT_PASS
sudo install -m644 webterminal.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now webterminal
```

Check it: `systemctl status webterminal`. Logs: `journalctl -u webterminal`.

> Very old Raspbian **Wheezy** (no systemd): start it from `/etc/rc.local`
> instead — see [DEPENDENCIES.md](DEPENDENCIES.md).

### Updating

Download the new binary, then:

```sh
sudo install -m755 webterminal-armv6 /usr/local/bin/webterminal
sudo systemctl restart webterminal
```

### Uninstall

```sh
sudo systemctl disable --now webterminal
sudo rm /etc/systemd/system/webterminal.service /usr/local/bin/webterminal
```

---

## Usage

### Command-line options

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `:8080` | Listen address, e.g. `:8081` or `192.168.1.10:8080` |
| `-shell` | `$SHELL` or `/bin/bash` | Shell to run (started as a login shell) |
| `-user` | env `WT_USER` | Basic auth username — **required** |
| `-pass` | env `WT_PASS` | Basic auth password — **required** |
| `-grace` | `5m` | How long a disconnected session keeps running before it is killed (`0` = immediately, `30s`, `1h`, …) |

The server refuses to start without credentials.

### The interface

| Control | What it does |
|---|---|
| Green/red dot (top left) | Connection status; reconnects automatically if the network drops |
| **Select** | Mobile copy: shows the terminal text as a plain page — long-press to select, or **Copy all** |
| **Copy** | Copies the current terminal selection (desktop: just drag-select — it auto-copies, PuTTY-style; `Ctrl+Shift+C` also works) |
| **Paste** | Pastes the clipboard (`Ctrl+V` also works; over plain HTTP the button opens a paste box — a browser security rule) |
| **A− / A+** | Font size (remembered per browser) |
| **⌨** | Toggles the shortcut key bar |

### Shortcut key bar

For phones/tablets and quick access: **Esc**, **Tab**, arrows, **Home/End**,
one-tap **Ctrl+C** / **Ctrl+D**, and sticky **Ctrl** / **Alt** — tap Ctrl
(it lights up), then press a letter: Ctrl→`r` = history search, Ctrl→`z` =
suspend, etc.

### Scrolling

- Desktop: mouse wheel or **Shift+PageUp/PageDown**.
- Mobile: swipe up/down on the terminal.
- Inside full-screen programs (`htop`, `less`, `vim`) there is no scrollback
  — that's how terminals work; use the program's own navigation keys.

### Tips

- **Selecting text inside `htop`/`vim`/`mc`**: those programs capture the
  mouse — hold **Shift** while dragging, or use the **Select** button.
- If the shell exits (`exit` or Ctrl+D), press **Enter** to start a new session.
- **Refreshing the page keeps your session**: the shell stays alive on the
  device for the `-grace` window (default 5 min) and the terminal re-attaches
  with recent output replayed. Long-running commands keep running meanwhile.
- Multiple browser tabs = multiple independent shell sessions (each tab has
  its own session; a duplicated tab shares one and steals the attachment).
- After updating, hard-refresh the page (**Ctrl+F5**) so the browser drops
  its cached UI.

---

## Build from source

Needs only Go ≥ 1.21 on your PC — nothing on the Pi, no Node.js, no Docker:

```sh
git clone https://github.com/muratbilge/webterminal.git
cd webterminal
make armv6            # -> webterminal-armv6 (all 32-bit Pis)
make arm64            # -> webterminal-arm64 (64-bit OS)
make local            # -> webterminal (your PC's architecture)
```

One-step deploy to a Pi over SSH: `make deploy PI=pi@192.168.1.50`

On Revolution Pi, ports 80/443 are used by the built-in web services
(piCtory/status pages); the default port 8080 avoids the conflict.

---

## Security notes — read this

- Credentials and all terminal traffic travel in **plain HTTP**. This is only
  acceptable on a trusted, isolated network (typical air-gapped plant LAN).
  Anyone who can sniff the network segment can capture the password and
  session.
- Do **not** expose port 8080 to the internet or an untrusted VLAN. If remote
  access is needed, put it behind a VPN (WireGuard runs fine even on a Pi 1)
  or a TLS reverse proxy (Caddy/nginx).
- The shell runs as the `User=` from the service file. Running as `root`
  means the web password *is* a root password — prefer a normal user.

## Compatibility problems this design avoids

- **ARMv6 trap**: most prebuilt tools (and Docker images) are ARMv7-only
  and crash with "illegal instruction" on Pi 1/Zero/RevPi Core 1. This binary
  is built with `GOARM=6`.
- **Node.js**: official Node builds dropped ARMv6 years ago, which rules out
  Wetty and similar Node-based terminals on old Pis. No Node here.
- **Docker on ARMv6**: most images no longer publish `arm/v6` variants.
  Native binary + systemd instead.
- **Old glibc**: static build (`CGO_ENABLED=0`) has no libc dependency.
- **Offline networks**: all frontend assets (xterm.js 5.5) are embedded in the
  binary — no CDN, works with no internet access.
- **Low RAM**: the server idles at ~10 MB, comfortable on 512 MB devices.
