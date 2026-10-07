# Using synchronize

A working guide to `syn` — for people, and for the AI assistants they ask to
operate it. The [README](README.md) says what the tool is; this says how to
drive it, what it will refuse to do, and where it will surprise you.

If you are an assistant acting on someone's machine, read
[Rules for assistants](#rules-for-assistants) before running anything. Two of
them prevent handing a stranger a shell on the user's server.

---

## The mental model

syn is a wrapper around `git`, run on a timer. That is the whole idea, and
everything follows from it:

- Your repositories stay ordinary git repositories. Any git client sees
  ordinary commits. syn adds no format of its own.
- Anything syn does, you could have typed. When it goes wrong, `git status`
  in the folder tells you the truth.
- It commits on your behalf, so it is careful about saying what went into
  each commit, and it takes a backup branch before rewriting anything.

Three separate jobs, each switchable:

| | |
|---|---|
| **at login** | bring down what changed while you were away |
| **on a timer** | keep up while you work — every 10 minutes by default |
| **at shutdown** | publish, and hold the shutdown until it has |

---

## Install

**Windows** — run `syn-setup.exe` from the
[releases](https://github.com/FalconTechDev/synchronize/releases). No administrator
rights. It puts `syn` on PATH and installs git if you have none.

**Linux / macOS workstation**

```sh
tar xzf syn-linux-amd64.tar.gz
./install.sh              # ~/.local/bin, or --system for /usr/local/bin
```

**Linux server** — see [On a server](#on-a-server).

**From source** — Go 1.25+ and git 2.31+:

```sh
go build -o bin/syn ./cmd/syn/
```

---

## The first five minutes

```bash
syn auth login              # paste a GitHub token with the `repo` scope
syn repo add owner/name     # clone it and start tracking
syn auto on                 # let it keep up by itself
syn status all              # check on it
```

That is a complete setup. Everything below is refinement.

### Naming repositories

**Every command that touches a repository needs it named.** `syn push` on its
own is refused; `syn push my-repo` or `syn push all` works. This is
deliberate: a bare `push` would publish work from a repository you were not
thinking about.

`syn ls` lists what is tracked. `syn path my-repo` prints just the folder, so
`cd $(syn path my-repo)` works.

---

## Commands by task

### Signing in

```bash
syn auth login                # interactive
syn auth login --pat <token>  # non-interactive
syn auth status
syn auth logout
```

Classic tokens need the `repo` scope. Fine-grained tokens need contents
read/write granted per repository.

### Tracking repositories

```bash
syn init [path]                          # register a clone you already have
syn repo add owner/name                  # clone from GitHub and track
syn repo add owner/name --path ./dir     # into a folder you choose
syn repo add owner/name --branch dev     # a second copy on another branch
syn repo add https://gitlab.com/g/t.git  # any git remote
syn repo create my-project [--public]    # create on GitHub, then clone
syn ls                                   # what syn tracks
syn remove <repo>                        # stop tracking; files stay
```

### Syncing by hand

```bash
syn pull <repo|all> [--force]     # --force resets hard to the remote
syn push <repo|all> [--force]     # commits your work first
syn sync <repo|all>               # pull, then push
syn status <repo|all> [--fetch]
syn diff <repo|all>               # uncommitted, untracked included
syn diff <repo> --pushed          # what the remote does not have
syn undo <repo>                   # take back syn's last commit, keep changes
syn log [repo] [n] [--live]
```

`--dry-run` on `pull`, `push` or `sync` shows what would happen and does
nothing. Worth having for a command that commits on your behalf.

### Automatic syncing

Three switches, independent:

```bash
syn auto                     # which are on
syn auto login on|off        # sync once at login
syn auto watch on|off        # then keep syncing while you work
syn auto watch 30m           # ...and how often
syn auto shutdown on|off     # publish before the machine goes down
syn auto on|off              # all three
syn auto <repo> on|off       # leave one repository out
syn pause <repo> 2h          # ...for a while only
syn start                    # run the watcher now
```

It uses the OS scheduler (Task Scheduler, systemd, launchd), so it survives a
reboot without syn running.

### Part of a repository

```bash
syn files <repo>                    # what is being synced
syn files include <repo> "src/**"   # only what matches
syn files exclude <repo> "logs/**"  # everything except
syn files clear <repo>              # the whole thing again
```

This is git sparse-checkout: excluded files leave your folder but stay on the
remote, and syn will not commit them as deleted.

To keep a file **on disk but out of automatic commits** — a local `.env` —
that is a different thing:

```bash
syn ignore my-repo "*.env"
syn ignore my-repo          # what is held back
syn ignore my-repo clear
```

### Conflicts

When a sync cannot merge cleanly, syn aborts it, takes a backup branch,
notifies you, and stops syncing that repository until you say what to do.

**The conflict markers are already in your files at that point** — the failed
sync put them there, and the index is unmerged. You do not need `resolve` to
see them.

```bash
syn status my-repo        # which files, and where the backup went
# edit them: keep what you want, delete the <<<<<<< ======= >>>>>>> lines
syn resolve my-repo       # sees the markers are gone, finishes and syncs
```

Or decide without editing:

```bash
syn resolve <repo> --ours     # keep your side everywhere
syn resolve <repo> --theirs   # keep the remote side
syn resolve <repo> --abort    # walk away from the merge or rebase
syn backups <repo>            # safety branches taken before rewrites
syn restore <repo> <branch>   # move back onto one
syn prune <repo|all> --days 30
```

`resolve` on a repository whose markers are still present tells you so and
changes nothing, so running it early is safe.

---

## On a server

A server is not a machine you work on. Say so, or syncing will commit the
logs and caches the running service writes and push them to the machines you
do work on.

```bash
syn deploy my-app on      # changes come down; nothing ever goes up
syn deploy                # which repositories are deploy targets
```

After that, `syn push` there explains itself instead of publishing, and a
sync pulls and stops. A conflict is still a conflict — being a deploy target
does not make it safe to discard an edit somebody made on the server.

### Running your program

Two commands, and picking the wrong one is the most common mistake.

**`syn restart` — hand over to a service manager you already have.** It runs
a command and *waits for it*:

```bash
syn restart my-bot bot.py --systemd my-bot
syn restart my-bot bot.py -- systemctl restart my-bot
syn restart my-bot run        # try it now, without syncing
syn restart my-bot off
```

**`syn app` — syn is the service manager.** For the program itself, which
never returns:

```bash
syn app my-bot -- python bot.py
syn app my-bot --watch "bot.py,src/**" -- python bot.py
syn app my-bot start          # run it here, attached, to try it
syn app my-bot off
```

Do not put a long-running program in `restart` or in a hook. Both wait for
the command to finish, so a bot would hold the sync until the hook timeout
(300s) killed it.

An app runs for as long as the watcher does. syn puts its output in the log,
brings it back if it falls over, and gives up after five starts that die
immediately — saying so once rather than retrying forever.

### Running from boot

`syn auto on` installs a **user** service, which on Linux only runs while its
owner has a session. A server needs one that starts at boot with nobody
logged in:

```bash
sudo syn service install --as deploy    # the account owning the repositories
syn service status
sudo syn service uninstall
journalctl -u synchronize -f
```

The unit is sandboxed to its own configuration and the repositories it
tracks. **That list is fixed when the unit is written**, so after
`syn repo add` on a server, run `sudo syn service install` again or the
service cannot write to the new folder.

On Windows there is no separate service: the scheduled task `syn auto on`
creates already runs from boot.

### Only deploy what CI passed

```bash
syn set my-app require_checks on
syn pull my-app --no-checks     # ...unless you say otherwise, by hand
```

It reads both of GitHub's systems — commit statuses and Actions check runs. A
failure outranks something still running, and if GitHub cannot be reached the
pull stops rather than assume the best. `--no-checks` is never set by the
watcher.

### Pinning and rolling back

```bash
syn pull my-app --to v1.4.2     # this release, on every machine
syn pull my-app --to v1.4.1     # that one was bad; go back
```

A backup branch is taken first. A bare branch name means the remote's:
`--to main` is the remote's main, not this machine's copy.

### A whole deployment

```bash
syn repo add me/my-bot --path /srv/my-bot
syn deploy my-bot on
syn set my-bot require_checks on
syn set my-bot interval 1m
syn app my-bot -- python bot.py
sudo syn service install --as deploy
```

Push to `main`; when CI is green the server takes it within the minute and
restarts the bot on the new code.

---

## Hooks

A hook runs a command **on this machine** after a sync actually changed
something.

```bash
syn hook add my-bot after-pull "systemctl restart my-bot"
syn hook add my-bot after-pull --paths "src/**,go.mod" "systemctl restart my-bot"
syn hook ls
syn hook run my-bot after-pull      # try it without syncing
syn hook clear my-bot [event]
syn hook timeout my-bot 120
syn hook rollback my-bot on|off
```

Events are `after-pull`, `after-push` and `on-conflict`. They fire only when
the sync changed something, so a bot is not restarted every ten minutes for
nothing.

The hook is told what happened:

| | |
|---|---|
| `SYN_EVENT` | `after-pull`, `after-push` or `on-conflict` |
| `SYN_REPO` / `SYN_PATH` / `SYN_BRANCH` | which repository, where, on what branch |
| `SYN_COMMIT` / `SYN_PREV_COMMIT` | where it is now, where it started |
| `SYN_CHANGED_FILES` | the paths, **one per line** |
| `SYN_CHANGED_COUNT` | how many |

One path per line because a comma is legal in a filename and a newline is
not:

```sh
echo "$SYN_CHANGED_FILES" | grep -q "^src/" || exit 0
```

`syn hook rollback on` puts a failed after-pull back where it started, so the
version that was running keeps running. It stands down when the working tree
is dirty, because a hard reset would take those changes with it. An
after-push hook is never rolled back — those commits are already published.

---

## Security model

**syn never takes a command from a repository.** Hook commands, `restart`
commands and `app` commands all live in `~/.synchronize/config.json` on the
machine that runs them. They never travel through git.

This is the single most important property of the tool, and it is not an
accident. If syn executed a script out of the repository — a `deploy.sh`, a
`commands.txt` — then everyone with push access would have a shell on every
server that pulls it. A push would become remote code execution.

So: **a push decides *whether* something runs. It never decides *what*
runs.** The administrator of the machine decides that, once, locally.

You can still defeat this yourself by pointing a hook at a script in the
repository:

```bash
syn hook add my-app after-pull "./deploy.sh"     # deploy.sh comes from git
```

Now anyone who can push can change `deploy.sh`. That is a decision to trust
everyone with push access — fine for your own repository on your own server,
wrong for anything public or shared. The safe shape is a fixed command:

```bash
syn hook add my-app after-pull --paths "site/**" \
    "cp -r site/* /var/www/html/ && nginx -s reload"
```

Now a contributor can change the site, but not what happens to it.

Other properties:

- Tokens live in `~/.synchronize/credentials.json`, mode 0600.
- They are never written into a remote URL. They reach git through a
  credential helper for the length of one command.
- `GIT_TERMINAL_PROMPT=0`, so git never sits waiting for a password
  unattended.

---

## Configuration

`~/.synchronize/config.json`, or wherever `SYNCHRONIZE_HOME` points.

```bash
syn config                                # current settings
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
syn set my-repo require_checks on|off
```

Files under `SYNCHRONIZE_HOME`:

| | |
|---|---|
| `config.json` | repositories, hooks, apps, settings |
| `credentials.json` | tokens, mode 0600 |
| `synchronize.log` | everything syn has done |
| `state.json` | last sync times, recorded conflicts |
| `locks/` | inter-process locks |

Setting `SYNCHRONIZE_HOME` gives you an isolated instance — useful for
testing without touching your real setup.

---

## For scripts and agents

```bash
syn status all --json        # one object per repository
syn status all --porcelain   # name, state, ahead, behind, changed, branch, path
syn path my-repo             # just the folder
```

`--json` for `status`:

```json
[
  {
    "name": "w",
    "path": "/srv/w",
    "branch": "main",
    "tracking": "origin/main",
    "ahead": 0,
    "behind": 0,
    "dirty": false,
    "changed": 0,
    "auto": true
  }
]
```

`--json` also works on `pull`, `push` and `sync`, giving `repo`, `ok`,
`summary`, `committed`, `pulled`, `pushed`, `files`, `conflict`, `error`.

**Exit codes:** 0 on success, non-zero on any failure — including a feature
that is not finished yet, so a script can tell the difference.

`status` and `doctor` answer with their exit code too: non-zero when a
repository cannot be read or has a conflict open, zero when everything is in
order. Being ahead or behind is not a failure — that is the normal state
between syncs. So this does what it looks like it does:

```bash
syn status all --json > state.json || notify "syn needs attention"
```

**Non-interactive:** commands that ask a question refuse when there is nobody
to answer, rather than guessing. Pass `--yes`:

```bash
syn undo my-repo --yes
```

`--yes` only answers the question. `--force` is separate and additionally
waives the check that the commit was syn's own — a script cannot rewrite a
commit it was never meant to touch by passing `--yes` alone.

`--quiet` silences stdout; the log file is still written.

---

## Rules for assistants

If you are an AI operating syn on someone's machine:

1. **Never make syn execute a script that lives in the repository.** Not
   `deploy.sh`, not a `commands.txt`, not "a file with the steps in it". It
   turns push access into remote code execution on every machine that pulls.
   If the user asks for this, explain the consequence before doing it, and
   prefer a fixed command with `--paths`.

2. **Never put a long-running program in a hook or in `restart`.** Both wait
   for the command. Use `syn app` for anything that does not return.

3. **Name the repository in every command.** `syn push all` if you truly mean
   all of them. A bare `syn push` is refused by design; do not work around
   that by looping over `syn ls`.

4. **Mark servers as deploy targets** before starting a watcher on them:
   `syn deploy <repo> on`. Otherwise the service's own logs and caches are
   committed and pushed to the user's other machines.

5. **Do not pass `--force` or `--no-checks` on your own initiative.** They
   discard work and bypass a gate the user asked for. Ask.

6. **Prefer `--dry-run` first** for `pull`, `push` and `sync` when you are
   not certain what state the repository is in.

7. **Read the log rather than guessing:** `syn log <repo>` and
   `syn status <repo> --json` say what actually happened.

8. **Token handling.** Never print, echo or copy the contents of
   `credentials.json`. Use `syn auth status` to check who is signed in.

---

## Known limits and gotchas

Honest list. These are real, and knowing them saves an afternoon.

**The watcher ignores `pause` and `auto <repo> off`.** The scheduled task
runs `watch --all`, which selects every tracked repository; the per-repository
filter is applied by the tray and the desktop app but not by the watcher
loop. Pausing works for commands you type, not for background syncing.

**A recorded conflict does not stop later syncs.** The repository is left with
conflict markers in the files, and the next automatic sync will commit them
and push. Resolve conflicts promptly, or pause that repository by hand. This
contradicts what the README implies, and is the more dangerous of the two.

**Backup branches accumulate.** Every rewrite takes one. `syn prune <repo>
--days 30` clears them; `keep_backup_days` sets the default.

**`--paths` matching** understands `*`, `?`, `[...]` and a trailing `/**`. It
is not full globbing: `src/*/x.go` will not match `src/a/b/x.go`. A pattern
is also tried against the **bare filename**, so `"bot.py"` matches
`deep/nested/bot.py` as well as `bot.py` — convenient, and occasionally more
than you meant. A pattern that matches nothing is not an error: the hook
simply never fires, which looks exactly like the hook being broken.

**Windows hooks and quoting.** A hook is handed to `cmd /s /c` verbatim, so
its own quotes survive. Output is decoded from the console code page, so a
non-English error message reaches the log as text.

**The systemd unit's writable paths are fixed at install time.** Re-run
`sudo syn service install` after adding a repository.

**Nothing is signed.** Windows SmartScreen will call the installer an
unknown publisher.

**`syn app` programs live with the watcher.** Stop the watcher and the
program stops. That is intended — on a server, systemd supervises syn and syn
supervises the program — but it means `syn app my-bot start` in a terminal
ends when you close the terminal.

---

## Troubleshooting

**`syn doctor`** first. It checks git, the account, background syncing, and
any repository in trouble.

| Symptom | Look at |
|---|---|
| Nothing syncs in the background | `syn auto` — are the switches on? On Linux, `systemctl --user status synchronize` |
| A repository is stuck | `syn status <repo>` — a conflict stops it until resolved |
| A hook did not run | It fires only when the sync **changed** something, and only when its `--paths` matched. `syn hook run <repo> after-pull` runs it regardless |
| A hook failed and you cannot read why | It is in the log: `syn log <repo>` |
| `git fetch` hangs | Network or VPN. Network commands give up after 2 minutes; `SYN_NETWORK_TIMEOUT=30s` changes that |
| Permission denied on a server | The systemd sandbox — re-run `sudo syn service install` |
| "no repositories registered yet" | `syn init` inside a clone, or `syn repo add owner/name` |

The log is `~/.synchronize/synchronize.log`, or the journal when running as a
system service.

---

## Command index

```
auth login|logout|status              repo add|create|ls
init [path]                           ls / list
path <repo>                           remove <repo>
set <repo> <key> <value>              config [set <key> <value>]

pull <repo|all> [--force] [--to <ref>] [--dry-run] [--json]
push <repo|all> [--force] [--dry-run] [--json]
sync <repo|all> [--dry-run] [--json]
status <repo|all> [--fetch] [--json|--porcelain]
diff <repo|all> [--pushed]            undo <repo> [--yes] [--force]
log [repo] [n] [--live]

auto [login|watch|shutdown] [on|off]  auto <repo> on|off
pause <repo> <duration|off>           start
watch <repo|all> [--no-tray]

files <repo> [include|exclude|clear] <glob>
ignore <repo> [<glob>|clear]

resolve <repo> [--ours|--theirs|--abort]
backups <repo>                        restore <repo> <branch>
prune <repo|all> [--days N]

hook ls|add|run|clear|timeout|rollback
app <repo> [-- <cmd>|start|off] [--watch <globs>]
restart <repo> <files> -- <cmd> | --systemd <unit> | run | off
deploy <repo> [on|off]
service install|uninstall|status [--as <user>]

shell    tray    serve [--addr :7391]    doctor    version
```

Global flags: `--all`, `--quiet`, `-v`, `--yes`, `--dry-run`, `--json`.
