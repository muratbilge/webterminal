# webterminal

A browser-based terminal that runs **on** the Pi itself — a single static
binary with an embedded xterm.js interface. Built specifically to work on old
hardware: Raspberry Pi 1 / Zero (ARMv6), Revolution Pi (RevPi Core / Connect),
and anything newer.

- ~6.5 MB binary, ~10 MB RAM at idle, zero dependencies on the device
- Full terminal control: colors, resize, interactive programs (htop, nano, vim)
- Auto-reconnect, adjustable font size
- Copy & paste that works over plain HTTP: drag-select auto-copies
  (PuTTY-style), Ctrl+V pastes; Copy/Paste buttons in the top bar
- **Select** button: shows the terminal text as a plain page so mobile
  long-press selection works (xterm.js has no native touch selection)
- Shortcut key bar (toggle with ⌨): Esc, Tab, sticky Ctrl/Alt, arrows,
  Home/End, Ctrl+C, Ctrl+D — for phones/tablets and quick access
- HTTP Basic Auth (see security notes below)

Full dependency list, compatibility matrix, and troubleshooting:
**[DEPENDENCIES.md](DEPENDENCIES.md)**

## Which binary for which device?

| Device | OS | Binary |
|---|---|---|
| Pi 1, Pi Zero/Zero W, RevPi Core 1 (CM1) | Raspberry Pi OS 32-bit | `webterminal-armv6` |
| RevPi Core 3 / Connect (CM3/CM3+/CM4S) | Revolution Pi OS (32-bit armhf) | `webterminal-armv6` |
| Pi 2/3/4/5, Zero 2 | Raspberry Pi OS 32-bit | `webterminal-armv6` |
| Pi 3/4/5, Zero 2 | Raspberry Pi OS 64-bit | `webterminal-arm64` |

`webterminal-armv6` runs on **every** Pi with a 32-bit OS — ARMv6 instructions
execute fine on ARMv7/v8 CPUs. It is fully static (no glibc dependency), so it
also works on very old Raspbian images.

## Build (on your PC — Go required, nothing needed on the Pi)

```sh
make armv6          # -> webterminal-armv6
make arm64          # -> webterminal-arm64 (optional)
```

## Quick try

```sh
./webterminal -user admin -pass secret -addr :8080
# open http://<pi-address>:8080 and log in
```

Flags: `-addr` (default `:8080`), `-shell` (default `$SHELL` or `/bin/bash`),
`-user`/`-pass` (or env `WT_USER`/`WT_PASS`). The server refuses to start
without credentials.

## Install as a service

Edit `webterminal.service` first: set `User=` (which Unix account the shell
runs as) and change `WT_PASS`. Then:

```sh
scp webterminal-armv6 pi@<address>:/tmp/
scp webterminal.service pi@<address>:/tmp/
ssh pi@<address>
  sudo install -m755 /tmp/webterminal-armv6 /usr/local/bin/webterminal
  sudo install -m644 /tmp/webterminal.service /etc/systemd/system/
  sudo systemctl daemon-reload
  sudo systemctl enable --now webterminal
```

Or in one step from this directory: `make deploy PI=pi@<address>`.

On Revolution Pi, port 80/443 are used by the built-in web services
(piCtory/status pages); the default port 8080 avoids the conflict.

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

- **ARMv6 vs ARMv7**: many prebuilt tools (and Docker images) are ARMv7-only
  and crash with "illegal instruction" on Pi 1/Zero. This binary is built with
  `GOARM=6`.
- **Node.js**: official Node builds dropped ARMv6 years ago, which rules out
  Wetty and similar Node-based terminals on old Pis. No Node here.
- **Docker on ARMv6**: most images no longer publish `arm/v6` variants.
  Native binary + systemd instead.
- **Old glibc**: static build (`CGO_ENABLED=0`) has no libc dependency.
- **Offline networks**: all frontend assets (xterm.js 5.5) are embedded in the
  binary — no CDN, works with no internet access.
- **Low RAM**: works comfortably within 64 MB (`MemoryMax` in the unit file)
  on 512 MB devices.
