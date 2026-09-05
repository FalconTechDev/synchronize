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
syn pull <repo> --to v1.4.2      # go to one commit, tag or branch and stay there
syn push <repo|all> [--force]    # commit your work and publish it
syn sync <repo|all>              # pull, then push
syn status <repo|all> [--fetch]
syn diff <repo|all>              # what is not committed yet
syn diff <repo> --pushed         # what this branch has that the remote lacks
syn undo <repo> [--yes]          # take back syn's last commit, keep the changes
syn log [repo] [n] [--live]      # all of it, or one repository's
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

Commands that ask before doing something take `--yes`. With nobody at the
keyboard the question cannot be answered, so unasked they refuse rather than
guess — `syn undo my-repo --yes` is the way to mean it from a script.

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
syn hook rollback my-bot on|off
```

Events are `after-pull`, `after-push` and `on-conflict`. They fire only when
the sync changed something, so a bot is not restarted every ten minutes for
nothing. Hooks live in `~/.synchronize/config.json` and never travel through
git — a command can only ever come from the machine that will run it.

The hook is told what happened, so it can decide whether it cares:

| | |
|---|---|
| `SYN_EVENT` | `after-pull`, `after-push` or `on-conflict` |
| `SYN_REPO` / `SYN_PATH` / `SYN_BRANCH` | which repository, where, on what branch |
| `SYN_COMMIT` / `SYN_PREV_COMMIT` | where it is now, and where it started |
| `SYN_CHANGED_FILES` | the paths behind that, one per line |
| `SYN_CHANGED_COUNT` | how many, for the common `if` |

One path per line because a comma is a legal character in a filename.

You can leave that test to syn instead — a hook can name the paths it cares
about, and sits out any sync that did not touch them:

```bash
syn hook add my-bot after-pull --paths "src/**,go.mod" "systemctl restart my-bot"
```

### Deploying with it

Three things turn the above into something you can point at a server.

**Do not deploy what CI rejected.** With this on, a pull checks the commit's
GitHub status first — both commit statuses and Actions check runs — and
refuses while anything is red or still running:

```bash
syn set my-bot require_checks on
syn pull my-bot --no-checks      # ...unless you say otherwise, by hand
```

If GitHub cannot be reached, the pull stops rather than assume the best.

**Put it back if it does not come up.** An after-pull hook that fails can
undo its own pull, so the version that was running stays running:

```bash
syn hook rollback my-bot on
```

It stands down when the working tree has uncommitted changes, because a hard
reset would take them with it. An after-push hook is never rolled back — those
commits are on the remote and someone may already have them.

**Pin a release, or go back to one.** `--to` moves the repository to exactly
one ref and nowhere else:

```bash
syn pull my-bot --to v1.4.2      # this release, on every machine
syn pull my-bot --to v1.4.1      # that one was bad; back to the last good one
```

It takes a backup branch first, like any other rewrite, and a branch name
means the remote's — `--to main` is the remote's main, not this machine's
copy of it.

---

## On a server

A server is not a machine you work on, and syncing it both ways is wrong:
the running service writes logs, caches and an sqlite file into its own
working tree, and an ordinary sync would commit that lot and push it to every
machine you actually work on.

So say what the machine is for:

```bash
syn deploy my-bot on     # changes come down; nothing ever goes up
syn deploy                # which repositories are deploy targets
```

After that, a sync pulls and stops. Whatever the service writes stays on that
disk, uncommitted, and `syn push` there explains itself instead of publishing.
A conflict is still a conflict — being a deploy target does not make it safe
to throw away an edit somebody made on the server by hand.

### Running from boot

`syn auto on` installs a **user** service, which on Linux only runs while its
owner has a session. A server needs one that starts at boot with nobody
logged in:

```bash
sudo syn service install --as deploy   # the account that owns the repositories
syn service status
journalctl -u synchronize -f
```

The unit runs as that account, restarts on failure, is given write access to
its own configuration and the repositories it tracks and nothing else, and
logs to the journal rather than to a file. `sudo syn service uninstall`
removes it.

The unit names the folders it may write to, so after adding a repository on a
server, run `sudo syn service install` again — otherwise the service cannot
write to the new one.

On Windows there is no separate service to install: the scheduled task
`syn auto on` creates already runs from boot. For a machine nobody signs in
to, set that task to run whether the user is signed in or not.

### A deployment, end to end

```bash
syn repo add me/my-bot --path /srv/my-bot
syn deploy my-bot on                      # this machine only receives
syn set my-bot require_checks on          # and only what CI passed
syn hook add my-bot after-pull --paths "src/**,go.mod"     "systemctl restart my-bot"
syn hook rollback my-bot on               # if it will not come up, go back
sudo syn service install --as deploy
```

Push to `main` from your laptop; when CI goes green the server takes it
within the interval, restarts the bot, and puts the old version back if the
restart fails. To pin or undo a release by hand:

```bash
syn pull my-bot --to v1.4.2
```

### Conflicts

```bash
syn resolve <repo>            # you have edited the files; finish and sync
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
syn set my-repo require_checks on   # never pull a commit whose CI is not green
```

---

## How a conflict is handled

1. The merge or rebase is **aborted** the moment it goes wrong
2. A **backup branch** `syn/backup/<host>/<branch>/<timestamp>` is taken, so
   your work is safe before anything is rewritten
3. A **notification** tells you which repository needs you
4. **Syncing stops** for that repository until you say what to do — nothing is
   decided behind your back, and no half-merge is committed or pushed

The branch itself does not move, but your working tree is not left untouched:
the conflicting files already have git's markers in them, and the index is
unmerged. That is deliberate — the conflict is in front of you, in the editor
you were already in, rather than hidden until you ask for it.

```
<<<<<<< Updated upstream
what the remote had
=======
what you wrote
>>>>>>> Stashed changes
```

So the work is: edit, then say you are done.

```bash
syn status my-repo       # which files, and where the backup went
# edit them: keep what you want, delete the <<<<<<< ======= >>>>>>> lines
syn resolve my-repo      # sees the markers are gone, finishes and syncs
```

`resolve` on a repository whose markers are still there tells you so and
changes nothing, so running it early is safe. If you would rather not edit
anything, pick a side outright: `--ours` keeps yours, `--theirs` keeps the
remote, `--abort` walks away from the merge or rebase entirely.

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
