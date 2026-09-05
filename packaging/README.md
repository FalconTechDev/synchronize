# Packaging

How syn is turned into something people can install. Nothing here is needed to
build or run syn itself — `go build ./cmd/syn/` is enough for that.

| | |
|---|---|
| [`windows/`](windows) | NSIS installer: puts syn on PATH, installs git if missing, offers to switch on automatic syncing |
| [`unix/`](unix) | shell installer for Linux and macOS |

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
