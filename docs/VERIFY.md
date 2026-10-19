# Where the terminal animations come from

Every line of output in `docs/t-*.svg` and `docs/demo.svg` was captured from a
running binary, not written from memory of what the code ought to print.

That distinction earned itself. The first version of these animations was
written from the source, and checking it against a real session found
differences in most of them:

| Drawn | Actually prints |
|---|---|
| `registered "notes" -> origin (github.com/me/notes)` | `registered "notes" -> origin (<the remote URL>)` |
| `  would pull 2 commits` / `would commit 3 files and push them` | `my-notes` / `  would commit 2 files:` / `      modified  todo.md` |
| `nothing uncommitted - everything is published` | `(nothing uncommitted)` |
| `my-notes: *.env kept out of commits` | `my-notes: *.env held back from automatic commits` |
| `big-repo: the whole repository again` | `big-repo: all file rules cleared (all files will be synced)` |
| `syn auto on` → three lines | three lines, then the repositories it covers, then four lines of usage |
| `logged in as Falcon (FalconTechDev)` | `logged in as FalconTechDev` |
| output with no prefix | `2026-10-19 07:42:09 [INFO] ` in front of every log line |

The last one is the big one: syn stamps the time and the level on everything
it logs, and drawing output without that made the pictures look like a
different program.

## What is still a substitution

Not everything can be captured verbatim and still read well, so three things
are swapped for something neutral, and nothing else is:

- **Hostnames and paths.** `DESKTOP-ARR5HDF` becomes `desktop`, and the
  temporary folders a test run uses become `C:\Users\me\...` or `/srv/my-bot`.
- **Repository names.** `my-notes` and `my-bot` throughout, rather than
  whatever the sandbox was called.
- **Commit hashes and timestamps.** Short hashes and the clock are from a real
  run, but they are not from the run that produced the line beside them.

The wording, the spacing, the order of lines, the prefixes and the exit
behaviour are all exactly what the tool does.

## Checking it again

```sh
go build -o bin/syn ./cmd/syn/
python docs/terminals.py          # rebuild the SVGs from the lines in that file
```

To re-verify against a running binary, point `SYNCHRONIZE_HOME` at a scratch
folder, run the commands from each screen, and compare. `syn service install`
is the one screen that cannot be captured on Windows; those two lines come
from the format strings in `internal/syn/service_linux.go`.
