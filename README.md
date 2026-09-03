# synchronize

**Auto-sync your git repos across all your machines — pull on startup, push on shutdown, zero friction.**

```
syn watch          # start the sync daemon with tray icon
syn pull           # pull all repos right now
syn push           # push all repos right now
syn status         # see what's going on
```

---

## What it does

`synchronize` (`syn`) is a thin layer on top of `git` that keeps your repositories in sync automatically. You work with files — `syn` handles the rest.

- **On startup** → pulls latest changes from every registered repo  
- **On shutdown** → auto-commits your work and pushes it  
- **Every N minutes** → background sync while you work  
- **On conflict** → backs up your work, notifies you, lets you resolve manually  
- **Tray icon** → quick Pull/Push/status without opening a terminal  
- **Web dashboard** → `syn serve` opens a local UI in the browser  

The git history stays completely standard — open any repo in GitHub, VS Code, or any git GUI.

---

## Install

**Prerequisites:** Go 1.23+, git 2.31+

```bash
git clone https://github.com/FalconTechDev/synchronize
cd synchronize
go build -o bin/syn.exe ./cmd/syn/   # Windows
go build -o bin/syn     ./cmd/syn/   # Linux / macOS
```

Add `bin/` to your PATH, then:

```bash
syn auth login          # connect your GitHub account
syn autostart install   # run on login + push on shutdown
```

---

## Commands

### Auth
```bash
syn auth login              # login with a Personal Access Token
syn auth login --pat <tok>  # non-interactive
syn auth status             # who am I logged in as
syn auth logout
```

### Repos
```bash
syn repo add owner/repo              # clone & register a repo
syn repo add owner/repo --path ./dir # use existing local copy
syn repo create my-project           # create new GitHub repo + clone
syn repo create my-project --public
syn repo ls                          # list your GitHub repos
```

### Sync
```bash
syn pull [repo]         # pull (stashes dirty work, restores after)
syn push [repo]         # auto-commit + rebase + push
syn sync [repo]         # pull then push
syn watch               # background daemon + tray icon
syn watch --no-tray     # daemon only (for servers / autostart)
```

### Status & diff
```bash
syn status              # all repos: branch, ahead/behind, last sync
syn diff [repo]         # what changed since last sync
syn log --live          # live-streamed colored log
syn log --lines 100
```

### Conflicts
```bash
syn resolve <repo>          # continue after fixing conflict markers
syn resolve <repo> --ours   # take your version automatically
syn resolve <repo> --theirs # take remote version automatically
syn resolve <repo> --abort  # give up, go back to before
```

### File transfer
```bash
# Send a file to another machine instantly
syn send report.pdf                    # creates a tmp repo automatically
syn send report.pdf @my-repo          # drop into a registered repo
syn receive my-repo --dest ./incoming  # watch for incoming files
```

### Temporary repos
```bash
syn tmp create                 # create a private throwaway repo
syn tmp create transfer-files  # named
syn tmp ls                     # list all tmp repos
syn tmp drop transfer-files    # delete from GitHub + unregister
```

### Encryption
```bash
syn encrypt init my-repo       # enable AES-256-GCM encryption
                               # prints key to share with other machines
syn encrypt add-key my-repo <key>  # import key on another machine
syn encrypt show-key my-repo       # print the key
```

### Notifications
```bash
syn notify "message"    # send a test OS toast notification
```
Native desktop notifications (Windows toast / macOS / Linux notify-send) fire automatically on conflicts and syncs.

### Dashboard
```bash
syn serve               # open http://127.0.0.1:7391
syn serve --addr :8080
```

### Autostart
```bash
syn autostart install    # pull on login, push on shutdown
syn autostart uninstall
syn autostart status
```

### Backups
```bash
syn backups [repo]           # list backup branches
syn resolve restore <branch> # restore a backup
syn prune --days 30          # delete backups older than 30 days
```

### File rules (sparse checkout)
```bash
syn files include my-repo "*.py" "docs/**"
syn files exclude my-repo "*.log" "node_modules/"
syn files show my-repo
```

---

## Configuration

Config lives at `~/.synchronize/config.json`. Edit directly or use:

```bash
syn config set interval_minutes 10   # sync every 10 min (default: 5)
syn config set strategy merge         # "rebase" (default) or "merge"
syn config set backup true            # backup branches before rewrites
syn config set notify_sync true       # toast on every sync, not just conflicts
syn config set commit_template "sync: {host} {time}"
```

Telegram notifications:
```bash
syn config set telegram.token  <bot-token>
syn config set telegram.chat_id <chat-id>
syn config set notify always    # "never" | "conflict" | "always"
```

---

## How conflicts are handled

1. `syn` detects the conflict and immediately **aborts** the merge/rebase  
2. Creates a **backup branch** `syn/backup/<host>/<branch>/<timestamp>` — your work is safe  
3. Sends a **desktop notification** (and Telegram if configured)  
4. Leaves your working tree clean so you can work normally  

To resolve:
```bash
syn resolve my-repo           # replays the conflict, puts markers in your files
# fix the markers in your editor
syn resolve my-repo           # finishes and syncs
```

---

## Security

- Tokens stored in `~/.synchronize/credentials.json` (mode 0600)  
- Never embedded in remote URLs — injected via git credential helper at runtime  
- Encryption keys in `~/.synchronize/keys/` (mode 0700), never committed  
- `GIT_TERMINAL_PROMPT=0` — git never prompts for credentials unattended  

---

## License

MIT
