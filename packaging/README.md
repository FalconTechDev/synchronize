# Packaging

How syn is turned into something people can install. Nothing here is needed to
build or run syn itself — `go build ./cmd/syn/` is enough for that.

| | |
|---|---|
| [`windows/`](windows) | NSIS installer: puts syn on PATH, installs git if missing, offers to switch on automatic syncing |
| [`unix/`](unix) | shell installer for Linux and macOS |
| [`server/`](server) | Linux server tarball: cross-build from Windows, install to `/usr/local/bin`, optional systemd unit |

The release workflow runs both, so what you build here is what ships.

## Windows

Needs [NSIS](https://nsis.sourceforge.io) — `winget install NSIS.NSIS`.

```powershell
.\windows\build.ps1 -Version 1.0.3     # writes dist\syn-setup.exe
```

The installer needs no administrator rights: it installs into
`%LOCALAPPDATA%\Programs\syn` and only ever writes under `HKCU`.

## Linux and macOS

```sh
./unix/install.sh              # for you only, into ~/.local/bin
./unix/install.sh --system     # for everyone, into /usr/local/bin
./unix/install.sh --uninstall
```

Run inside a clone it builds from source; run beside a released binary it
installs that instead. Automatic syncing is set up by syn itself afterwards
(`syn auto on`) — through systemd on Linux, launchd on macOS — so the
installer does not have to know about either.

## Linux server (from Windows)

When you only have `syn.exe` on your laptop, build a Linux binary and ship
it to the server:

```powershell
.\packaging\server\build.ps1              # dist\syn-server-linux-amd64.tar.gz
.\packaging\server\build.ps1 -Arch arm64  # for ARM servers (e.g. Raspberry Pi)
```

On the server:

```sh
tar xzf syn-server-linux-amd64.tar.gz
sudo ./install.sh --service --as deploy
```

Then as the deploy user: `syn auth login`, `syn repo add`, `syn deploy on`.
See the main README section "On a server" for the full deployment flow.
