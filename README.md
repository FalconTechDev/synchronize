# synchronize

**Keep your git repositories in step across every machine you work on.**

You sit down, your work is already there. You walk away, it is published
before the machine goes down — the shutdown waits for it.

```bash
syn auto on              # sync at login, on a timer, and before shutdown
syn status all           # what would a sync do right now
syn sync my-repo         # do it now
```

The history stays ordinary git — open the repository in GitHub, VS Code or any
git client and the commits look like commits. It does push safety branches
under `syn/backup/` before rewriting anything, so the branch list is busier
than a hand-tended repository; `syn prune` clears the old ones.

---

## What it does

- **At login** — brings in what changed while you were away
- **On a timer** — keeps up while you work, every 10 minutes by default
- **At shutdown** — commits and publishes, and holds the shutdown until it has
- **On conflict** — takes a backup branch, stops syncing that repository and
  tells you, rather than merging something half-decided
- **Hooks** — runs a command of yours after a sync actually changed something

Each of those is a separate switch. None of them is forced on you.

---

## Install

Everything is one binary. Take it from the
[releases](https://github.com/FalconTechDev/synchronize/releases).

**Windows** — run `syn-setup.exe`. It installs git if you do not have it, puts
`syn` on your PATH, and offers to switch on automatic syncing. No
administrator rights needed.

**Linux and macOS** — the tarball carries an installer beside the binary:

```sh
tar xzf syn-linux-amd64.tar.gz
./install.sh            # into ~/.local/bin, or --system for /usr/local/bin
```

Then, wherever you are:

```bash
syn auth login           # connect your GitHub account
syn repo add owner/name  # clone it and start tracking
syn auto on              # let it keep up by itself
```

### From source

**Needs:** Go 1.25+, git 2.31+

```bash
git clone https://github.com/FalconTechDev/synchronize
cd synchronize
go build -o bin/syn.exe ./cmd/syn/   # Windows
go build -o bin/syn     ./cmd/syn/   # Linux / macOS
```

The installer recipes live in [`packaging/`](packaging); they are not needed to
build or run syn itself. The release workflow runs those same scripts, so what
you build locally is what ships.

---

## Commands

Anything that touches a repository needs it named. Pass `all` to mean every
repository you track — a bare `syn push` will not quietly publish work from
one you were not thinking about. Pausing a repository only keeps it out of
automatic syncing; commands you type yourself still reach it.

### Signing in

```bash
syn auth login               # paste a Personal Access Token
syn auth login --pat <tok>   # or give it directly
syn auth status
syn auth logout
```

The token needs the `repo` scope. Classic tokens work; fine-grained ones need
the permissions granted explicitly per repository.

### Repositories

```bash
syn repo add owner/name                 # clone it and track it
syn repo add owner/name --path ./dir    # into a folder you choose
syn repo add owner/name --branch dev    # a second copy on another branch
syn repo add https://gitlab.com/g/t.git # any git remote, not only GitHub
syn repo create my-project [--public]   # make it on GitHub, then clone
syn repo ls                             # what you have on GitHub
syn ls                                  # what syn is tracking
syn remove <repo>                       # stop tracking; files stay put
```

### Syncing

```bash
syn pull <repo|all> [--force]    # bring changes in (--force resets to remote)
syn push <repo|all> [--force]    # commit your work and publish it
syn sync <repo|all>              # pull, then push
syn status <repo|all> [--fetch]
syn diff <repo|all>              # what is not committed yet
syn diff <repo> --pushed         # what this branch has that the remote lacks
syn undo <repo>                  # take back syn's last commit, keep the changes
syn log [n] [--live]
```

Add `--dry-run` to `pull`, `push` or `sync` to see what they would do without
doing it — for a command that commits on your behalf, worth having.

### For scripts

```bash
syn status all --json        # one object per repository
syn status all --porcelain   # name, state, ahead, behind, changed, branch, path
syn path my-repo             # just the folder, so cd $(syn path my-repo) works
```

Anything that fails exits non-zero, including a feature that is not finished
yet, so a script can tell the difference.

Both directions list the files behind the commits, so an automatic commit
never happens without telling you what went into it. `diff` counts untracked
files too, because an automatic commit sweeps those in as well.

### Syncing by itself

Three switches you can set independently:

```bash
syn auto                     # which of them are on
syn auto login on|off        # sync once when you log in
syn auto watch on|off        # after that, keep syncing while you work
syn auto watch 30m           # same, and how often (30m, 12h, 1d)
syn auto shutdown on|off     # publish before the machine goes down
syn auto on|off              # all three
syn auto <repo> on|off       # leave one repository out of it
syn pause <repo> 2h          # the same, but only for a while
syn pause <repo> off         # end the pause early
```

Pausing only affects automatic syncing. Commands you type yourself always
reach the repository, so work is never stranded.

It uses the OS scheduler, so it survives a reboot without syn running.

### Hooks

Run something of yours once a sync has actually changed something — restarting
a bot when new code lands, say:

```bash
syn hook add my-bot after-pull "systemctl restart my-bot"
syn hook ls
syn hook run my-bot after-pull      # try it without syncing
syn hook clear my-bot [event]
syn hook timeout my-bot 120
```

Events are `after-pull`, `after-push` and `on-conflict`. They fire only when
the sync changed something, so a bot is not restarted every ten minutes for
nothing. Hooks live in `~/.synchronize/config.json` and never travel through
git — a command can only ever come from the machine that will run it.

### Conflicts

```bash
syn resolve <repo>            # replay the conflict into your working tree
syn resolve <repo> --ours     # keep your side everywhere
syn resolve <repo> --theirs   # keep the remote side
syn resolve <repo> --abort    # give up on the rebase or merge
syn backups <repo>            # safety branches taken before rewrites
syn restore <repo> <branch>   # move back onto one
syn prune <repo|all> --days 30
```

### Part of a repository

```bash
syn files <repo>                        # what is being synced
syn files include <repo> "src/**"       # only what matches
syn files exclude <repo> "logs/**"      # everything except
syn files clear <repo>                  # the whole thing again
```

`files` decides what is checked out. To keep something on disk but out of the
automatic commits — a local `.env`, say — use `ignore`:

```bash
syn ignore my-repo "*.env"
syn ignore my-repo          # what is being held back
syn ignore my-repo clear
```

This is git sparse-checkout: what you leave out is removed from your folder
but stays on the remote, and syn will never commit it as deleted.

### Checking on it

```bash
syn doctor    # git, account, background syncing, and any repository in trouble
```

---

## Ways to look at it

Besides the commands, there are three:

**A session that stays open.** `syn shell` keeps a prompt until you type
`exit`, completing commands and repository names on Tab and showing, under the
prompt, whatever the command still expects.

**In the tray.** `syn tray` sits by the clock with the current state and
Pull/Push to hand.

**In a browser.** `syn serve` puts a dashboard on `http://localhost:7391` —
every repository, what is ahead or behind, and the log as it happens. Take a
different port with `--addr :8080`.

There is also a desktop application in [`cmd/syn-app`](cmd/syn-app), built with
[Wails](https://wails.io). It is younger than the rest and not in the releases
yet; build it with `wails build` from that folder.

---

## Not finished yet

These have code behind them but are switched off, because they are not
trustworthy enough yet to hand your files to:

| | |
|---|---|
| `syn send <file> @<repo>` | pass a file to another machine through a shared repo |
| `syn receive <repo>` | watch for files arriving and copy them out |
| `syn tmp <files...>` | one-shot transfer through a throwaway repository |
| `syn encrypt <repo>` | per-repository AES-256 via git clean/smudge filters |
| `syn config telegram ...` | conflict alerts to a Telegram bot |

Desktop notifications do work: a conflict raises one on Windows, macOS and
Linux without you asking for anything.

---

## Configuration

`~/.synchronize/config.json`, or:

```bash
syn config                                # what it is set to now
syn config set interval_minutes 30m       # or a plain number of minutes
syn config set strategy merge             # rebase (default) or merge
syn config set backup true                # branch before any rewrite
syn config set keep_backup_days 14
syn config set commit_template "sync: {host} {time}"
```

Per repository:

```bash
syn set my-repo interval 12h
syn set my-repo branch dev
syn set my-repo name shorter-name
```

---

## How a conflict is handled

1. The merge or rebase is **aborted** the moment it goes wrong
2. A **backup branch** `syn/backup/<host>/<branch>/<timestamp>` is taken, so
   your work is safe before anything is rewritten
3. A **notification** tells you which repository needs you
4. Nothing is half-merged: the branch is where it was, and syncing stops for
   that repository until you say what to do

Then, when you are ready:

```bash
syn resolve my-repo      # replays it, putting conflict markers in the files
# edit the files: keep what you want, delete the <<<<<<< ======= >>>>>>> lines
syn resolve my-repo      # sees the markers are gone, finishes and syncs
```

---

## Security

- Tokens live in `~/.synchronize/credentials.json`, mode 0600
- They are never written into a remote URL — they reach git through a
  credential helper for the length of one command
- `GIT_TERMINAL_PROMPT=0`, so git never sits waiting for a password unattended
- Hook commands come from the local config, never from the repository, so
  whoever pushed last cannot run anything on your machine

---

## License

MIT
