<div align="center">

# synchronize

**Keep every git repository in step across every machine you work on.**

[![CI](https://github.com/FalconTechDev/synchronize/actions/workflows/ci.yml/badge.svg)](https://github.com/FalconTechDev/synchronize/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/FalconTechDev/synchronize)](https://github.com/FalconTechDev/synchronize/releases/latest)
[![Go 1.25](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green)](LICENSE)

[Install](#install) · [Quick Start](#quick-start) · [Commands](#commands) · [Deployment](#on-a-server) · [Full Guide](GUIDE.md)

</div>

---

You sit down — your work is already there.  
You walk away — it is published before the machine goes down.

```bash
syn auto on              # sync at login, every 10 minutes, and before shutdown
syn status all           # see what a sync would do right now
syn sync my-repo         # do it now, by hand
```

Repositories stay **ordinary git repositories**. Any git client, GitHub, VS Code — the commits look like commits. `syn` adds nothing of its own.

---

## What it does

| | |
|---|---|
| **At login** | Brings in everything that changed while you were away |
| **On a timer** | Keeps up while you work — every 10 minutes by default |
| **At shutdown** | Commits, publishes, and holds the shutdown until it is done |
| **On conflict** | Takes a backup branch, stops syncing that repo, and tells you |
| **Hooks** | Runs your command once a sync has actually changed something |

Each of those is a separate switch. None of them is forced on you.

---

## Install

### Windows

Download and run **[`syn-setup.exe`](https://github.com/FalconTechDev/synchronize/releases/latest)** from the latest release.

- Installs git if you do not have it
- Puts `syn` on your PATH
- No administrator rights required
- Offers to turn on automatic syncing right away

### Linux

```sh
tar xzf syn-linux-amd64.tar.gz
./install.sh              # installs to ~/.local/bin
# or: ./install.sh --system  to install into /usr/local/bin
```

### macOS

```sh
# Apple Silicon (M1/M2/M3)
tar xzf syn-macos-arm64.tar.gz
./install.sh

# Intel
tar xzf syn-macos-amd64.tar.gz
./install.sh
```

### From source

Requires **Go 1.25+** and **git 2.31+**:

```bash
git clone https://github.com/FalconTechDev/synchronize
cd synchronize
go build -o bin/syn ./cmd/syn/        # Linux / macOS
go build -o bin/syn.exe ./cmd/syn/    # Windows
```

---

## Quick Start

Five commands and you are done:

```bash
# 1. Connect your GitHub account
syn auth login

# 2. Clone a repository and start tracking it
syn repo add owner/repo-name

# 3. (optional) Add more repositories
syn repo add owner/another-repo

# 4. Turn on automatic syncing
syn auto on

# 5. Check the status
syn status all
```

That is a complete setup. `syn` will now sync at login, every 10 minutes while you work, and before your machine shuts down.

### Connecting with a token

`syn auth login` will prompt you for a GitHub Personal Access Token.

- **Classic token** — needs the `repo` scope
- **Fine-grained token** — needs _Contents: read/write_ per repository

You can also pass it directly:

```bash
syn auth login --pat ghp_your_token_here
```

The token is stored in `~/.synchronize/credentials.json` with mode 0600. It is never written into a remote URL.

---

## Commands

> Every command that touches a repository requires it to be **named**. Use `all` to mean every tracked repository. A bare `syn push` is refused by design — you decide which repository is being published.

### Authentication

```bash
syn auth login                # interactive: paste a Personal Access Token
syn auth login --pat <token>  # non-interactive
syn auth status               # who is signed in
syn auth logout
```

### Managing repositories

```bash
syn repo add owner/name                  # clone from GitHub and track
syn repo add owner/name --path ./dir     # into a specific folder
syn repo add owner/name --branch dev     # a second copy on another branch
syn repo add https://gitlab.com/g/t.git  # any git remote, not only GitHub
syn repo create my-project [--public]    # create on GitHub, then clone
syn repo ls                              # what you have on GitHub
syn ls                                   # what syn is tracking
syn remove <repo>                        # stop tracking; files stay
```

### Syncing by hand

```bash
syn pull <repo|all> [--force]     # bring changes in
syn pull <repo> --to v1.4.2       # go to this tag and stay there
syn push <repo|all> [--force]     # commit your work and publish it
syn sync <repo|all>               # pull, then push
syn status <repo|all> [--fetch]   # what would happen
syn diff <repo|all>               # uncommitted changes
syn diff <repo> --pushed          # what the remote does not have yet
syn undo <repo> [--yes]           # take back syn's last commit, keep changes
syn log [repo] [n] [--live]
```

Add `--dry-run` to `pull`, `push`, or `sync` to preview without doing anything.

### Automatic syncing

```bash
syn auto                       # which switches are on
syn auto login on|off          # sync once at login
syn auto watch on|off          # keep syncing on a timer
syn auto watch 30m             # set the interval (30m, 12h, 1d)
syn auto shutdown on|off       # publish before shutdown
syn auto on|off                # all three at once
syn auto <repo> on|off         # exclude one repository
syn pause <repo> 2h            # pause for a duration
syn start                      # run the watcher right now
```

Uses the OS scheduler (Task Scheduler on Windows, systemd/launchd on Unix). Survives a reboot.

### Hooks

Run a command whenever a sync actually changes something:

```bash
syn hook add my-bot after-pull "systemctl restart my-bot"
syn hook add my-bot after-pull --paths "src/**,go.mod" "systemctl restart my-bot"
syn hook ls
syn hook run my-bot after-pull      # try it without syncing
syn hook clear my-bot [event]
syn hook timeout my-bot 120
syn hook rollback my-bot on|off     # put the repo back if the hook fails
```

Events: `after-pull`, `after-push`, `on-conflict`.

Hook commands live in `~/.synchronize/config.json` on the machine that runs them. They **never travel through git**. A push decides *whether* something runs — never *what*.

---

## Conflicts

When a sync cannot merge cleanly, `syn` aborts it, takes a backup branch, notifies you, and stops syncing until you say what to do. The conflict markers are already in your files.

```bash
syn status my-repo       # which files, where the backup went
# fix the markers in your editor
syn resolve my-repo      # sees the markers are gone, finishes and syncs
```

Or decide without editing:

```bash
syn resolve <repo> --ours     # keep your side everywhere
syn resolve <repo> --theirs   # keep the remote side
syn resolve <repo> --abort    # walk away
syn backups <repo>            # list safety branches
syn restore <repo> <branch>   # move back onto one
syn prune <repo|all> --days 30
```

---

## On a server

Mark the repository as a deploy target so the running service's logs and caches do not get committed and pushed to your other machines:

```bash
syn deploy my-app on          # changes come in; nothing ever goes out
```

### CI gate

Only deploy commits that CI has approved:

```bash
syn set my-app require_checks on
```

Reads both commit statuses and Actions check runs. If GitHub is unreachable, the pull stops rather than assume the best.

### Auto-restart

```bash
# Delegate to an existing service manager
syn restart my-bot bot.py --systemd my-bot

# Let syn manage the process itself
syn app my-bot -- python bot.py
syn app my-bot --watch "bot.py,src/**" -- python bot.py
```

### Boot service (Linux)

```bash
sudo syn service install --as deploy   # starts at boot, no login required
syn service status
journalctl -u synchronize -f
```

### End-to-end example

```bash
syn repo add me/my-bot --path /srv/my-bot
syn deploy my-bot on
syn set my-bot require_checks on
syn set my-bot interval 1m
syn restart my-bot bot.py --systemd my-bot
sudo syn service install --as deploy
```

Push to `main` from your laptop. When CI goes green the server takes it within the minute, restarts the bot, and rolls back if the restart fails.

---

## Scripting

```bash
syn status all --json        # one JSON object per repository
syn status all --porcelain   # name state ahead behind changed branch path
syn path my-repo             # just the folder — cd $(syn path my-repo)
```

`--json` also works on `pull`, `push`, and `sync`. Non-zero exit on any failure.

Commands that ask before acting take `--yes` for non-interactive use. `--quiet` silences stdout; the log is still written.

---

## Configuration

```bash
syn config                                  # current settings
syn config set interval_minutes 30m
syn config set strategy merge               # rebase (default) or merge
syn config set backup true
syn config set keep_backup_days 14
syn config set commit_template "sync: {host} {time}"
```

Per repository:

```bash
syn set my-repo interval 12h
syn set my-repo branch dev
syn set my-repo name shorter-name
syn set my-repo require_checks on
```

Config lives in `~/.synchronize/config.json` (or wherever `SYNCHRONIZE_HOME` points).

---

## More ways to use it

| | |
|---|---|
| `syn shell` | Interactive shell with Tab completion and inline hints |
| `syn tray` | Sits in the system tray with Pull / Push to hand |
| `syn serve` | Dashboard at `http://localhost:7391` — all repos, log as it happens |
| `syn doctor` | Checks git, account, background syncing, and any repo in trouble |

---

## Security

- Tokens live in `~/.synchronize/credentials.json`, **mode 0600**
- Never written into a remote URL — passed through a credential helper, one command at a time
- `GIT_TERMINAL_PROMPT=0` — git never waits for a password unattended
- Hook commands come from local config, never from the repository — no push can run code on your machine without you setting it up first

See [GUIDE.md → Security model](GUIDE.md#security-model) for the full reasoning.

---

## Troubleshooting

Run `syn doctor` first. It checks git, the account, background syncing, and any repository in trouble.

| Symptom | Where to look |
|---|---|
| Nothing syncs in the background | `syn auto` — are the switches on? On Linux: `systemctl --user status synchronize` |
| A repository is stuck | `syn status <repo>` — a conflict stops it until resolved |
| A hook did not run | Only fires when the sync **changed** something and `--paths` matched. Try `syn hook run <repo> after-pull` |
| A hook failed silently | It is in the log: `syn log <repo>` |
| `git fetch` hangs | Network or VPN. Commands give up after 2 minutes. `SYN_NETWORK_TIMEOUT=30s` changes that |
| Permission denied on a server | The systemd sandbox — re-run `sudo syn service install` after adding a repo |

The log: `~/.synchronize/synchronize.log`

---

## Full reference

Everything above is the common path. [**GUIDE.md**](GUIDE.md) has the complete command index, every flag, the deployment setup end to end, the security model, and an honest list of known quirks.

---

## License

MIT — see [LICENSE](LICENSE).
