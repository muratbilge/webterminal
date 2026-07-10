# Dependencies & Requirements

Complete list of everything webterminal needs — on the Raspberry Pi, on the
build machine, and in the browser. The guiding principle of this project:
**the Pi itself needs almost nothing**, because the binary is fully static and
all web assets are embedded inside it.

---

## 1. On the Raspberry Pi (runtime)

### Required

| Dependency | Why | Notes |
|---|---|---|
| Linux kernel ≥ 3.2 | Minimum for Go-built binaries | Every Raspbian/Raspberry Pi OS since 2012 qualifies, including old wheezy images |
| A shell, default `/bin/bash` | The program the terminal runs | Preinstalled on every Raspberry Pi OS / Revolution Pi OS image. On stripped-down systems (Alpine, Buildroot) use `-shell /bin/sh` |
| Free TCP port (default 8080) | The web server | Change with `-addr :PORT` if occupied |

That is the entire required list. **No libc version matters** (the binary is
static, built with `CGO_ENABLED=0`), no Python, no Node.js, no web server, no
Docker, no package installation, no internet access.

### Required for running as a service

| Dependency | Why | Notes |
|---|---|---|
| systemd | `webterminal.service` unit | Present on Raspbian **Jessie (2015) and newer**, all Revolution Pi OS versions. |

On ancient **Raspbian Wheezy** (sysvinit, no systemd): skip the unit file and
start from `/etc/rc.local` instead:

```sh
WT_USER=admin WT_PASS=secret /usr/local/bin/webterminal -addr :8080 &
```

### Optional (nice to have on the Pi)

| Package | Why | Install |
|---|---|---|
| `ncurses-term` | Full terminfo for `xterm-256color`; Raspberry Pi OS already ships it. Only minimal distros lack it — symptom: wrong colors or broken keys in `htop`/`nano` | `sudo apt install ncurses-term` |
| `sudo` | Admin tasks when the service runs as a normal user (recommended over `User=root`) | preinstalled on Raspberry Pi OS |
| `htop`, `nano`, `vim` | The tools you'll actually use in the terminal | `sudo apt install htop nano vim` |

### Hardware / OS compatibility matrix

| Device | SoC / arch | OS | Binary | Works? |
|---|---|---|---|---|
| Raspberry Pi 1 A/B/A+/B+ | BCM2835, ARMv6 | Raspberry Pi OS 32-bit | `webterminal-armv6` | ✅ |
| Raspberry Pi Zero / Zero W | BCM2835, ARMv6 | Raspberry Pi OS 32-bit | `webterminal-armv6` | ✅ |
| RevPi Core 1 (CM1) | BCM2835, ARMv6 | Revolution Pi OS (armhf) | `webterminal-armv6` | ✅ |
| RevPi Core 3 / 3+ / Connect (CM3/CM3+) | BCM2837, ARMv8 in 32-bit mode | Revolution Pi OS (armhf) | `webterminal-armv6` | ✅ |
| RevPi Core S/SE, Connect 4 (CM4S/CM4) | BCM2711, ARMv8 | Revolution Pi OS (armhf) | `webterminal-armv6` | ✅ |
| Pi 2 | BCM2836, ARMv7 | 32-bit OS | `webterminal-armv6` | ✅ |
| Pi 3 / Zero 2 W / Pi 4 / Pi 400 / Pi 5 | ARMv8 | 32-bit OS | `webterminal-armv6` | ✅ |
| Pi 3 / Zero 2 W / Pi 4 / Pi 400 / Pi 5 | ARMv8 | 64-bit OS | `webterminal-arm64` | ✅ |
| Any x86 Linux box (like this dev server) | amd64 | any | `webterminal` (local build) | ✅ |

Rule of thumb: **32-bit OS → `webterminal-armv6`, 64-bit OS → `webterminal-arm64`.**
Check with: `getconf LONG_BIT` on the Pi.

### Resource footprint

- Binary: ~6.5 MB on disk
- RAM: ~10 MB idle + roughly 1–2 MB per open terminal session (plus whatever
  the shell/programs use)
- CPU: negligible; even a Pi 1 handles multiple sessions

---

## 2. On the build machine (your PC — not the Pi)

Cross-compilation means you never compile on the Pi.

| Dependency | Version used | Why | Notes |
|---|---|---|---|
| **Go** | 1.26.5 (≥ 1.21 fine) | Compiler | On this server: `/home/murat/.local/go-toolchain/bin/go`. Debian/Ubuntu: `sudo apt install golang-go` |
| `make` | any | Convenience build targets | Optional — the raw commands are below |
| `curl` | any | Only to (re-)download xterm.js assets | Only needed if you upgrade the frontend libs; current ones are committed in `web/vendor/` |
| `ssh`/`scp` | any | `make deploy` | Optional |

**Not needed anywhere:** Node.js, npm, Docker, gcc/cross-toolchains (pure-Go,
CGO disabled).

Raw build commands (what the Makefile runs):

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 go build -ldflags "-s -w" -o webterminal-armv6 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64          go build -ldflags "-s -w" -o webterminal-arm64 .
```

### Go module dependencies (fetched automatically by `go build`)

| Module | Version | Purpose |
|---|---|---|
| `github.com/gorilla/websocket` | v1.5.3 | Websocket server for the terminal stream |
| `github.com/creack/pty` | v1.1.24 | Spawns the shell inside a pseudo-terminal (resize support) |

Both are pure Go and work with `CGO_ENABLED=0`. Pinned in `go.mod`/`go.sum`.

### Vendored frontend assets (committed in `web/vendor/`, embedded into the binary)

| Asset | Version | Source |
|---|---|---|
| `xterm.js` + `xterm.css` | @xterm/xterm 5.5.0 | `cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0` |
| `addon-fit.js` | @xterm/addon-fit 0.10.0 | `cdn.jsdelivr.net/npm/@xterm/addon-fit@0.10.0` |

These never need to be re-downloaded unless you want to upgrade them.

---

## 3. In the browser (the client side)

Runs on the machine you browse *from*, so old Pis are unaffected.

- Any browser with ES2017 support: Chrome/Edge ≥ 63, Firefox ≥ 78,
  Safari ≥ 12, and current mobile browsers. No Internet Explorer.
- Websocket support (all of the above).
- No plugins, no internet access — everything is served by the Pi.
- Clipboard: over plain HTTP browsers block silent clipboard *reading*, which
  is why the Paste button falls back to a dialog; Ctrl+V and the copy
  functions work everywhere. Serving over HTTPS would remove this limit.

---

## 4. Network requirements

- One TCP port (default 8080) reachable from the client — plain HTTP +
  websocket upgrade on the same port.
- No outbound internet needed on the Pi, ever (works air-gapped).
- On Revolution Pi, ports 80/443 are taken by the built-in piCtory/status web
  apps — the 8080 default avoids that collision.
- If a firewall runs on the Pi: `sudo ufw allow 8080/tcp` (or the nftables
  equivalent).

---

## 5. Troubleshooting quick reference

| Symptom | Cause | Fix |
|---|---|---|
| `Illegal instruction` on Pi 1/Zero/RevPi Core 1 | Binary built for ARMv7+ | Use `webterminal-armv6` (built with `GOARM=6`) |
| `cannot execute binary file: Exec format error` | 64-bit binary on 32-bit OS (or vice versa) | `getconf LONG_BIT`, pick matching binary |
| `bind: address already in use` | Port taken | `-addr :8081` or find the culprit: `sudo ss -tlnp \| grep 8080` |
| `refusing to start without credentials` | No user/pass configured | Set `-user/-pass` or `WT_USER`/`WT_PASS` |
| Wrong colors / broken keys in htop or nano | Missing `xterm-256color` terminfo (minimal distros only) | `sudo apt install ncurses-term` |
| Browser shows login loop | Wrong credentials; some browsers cache a bad password | Close tab, reopen, or private window |
| Page loads but terminal dead, red dot | Websocket blocked by a proxy in between | Connect directly to the Pi's port; if a reverse proxy is used it must forward `Upgrade`/`Connection` headers |
| Old page behavior after an update | Browser cached `app.js` | Hard refresh (Ctrl+F5) |
| Can't select text inside `htop`/`vim`/`mc` | The program captures mouse events | Hold **Shift** while dragging, or use the **Select** button |
| Can't select text on a phone/tablet | xterm.js has no touch selection | Use the **Select** button (top bar), then long-press |
| Service dies on boot before network | — | Unit already has `After=network.target`; for Wi-Fi-only Pis use `systemctl enable systemd-networkd-wait-online` or `After=network-online.target` |

---

## 6. Security dependencies (deliberate choices, revisit if exposure changes)

Current setup: HTTP Basic Auth over **plain HTTP** — chosen for trusted,
isolated LANs. Nothing extra to install. If the network is not fully trusted:

- **VPN**: WireGuard runs on any Pi (`sudo apt install wireguard`), even Pi 1.
- **TLS reverse proxy**: Caddy (single Go binary, automatic self-signed certs)
  or nginx (`sudo apt install nginx`) in front of port 8080.
- Native TLS in webterminal itself is a straightforward future addition
  (Go's `http.ListenAndServeTLS`) — ask if you want it.
