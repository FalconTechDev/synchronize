#!/usr/bin/env python3
"""Builds the terminal animations the README uses.

Two rules, both learned the hard way.

**The text is real.** Every line below was captured from the tool, not
written from memory of what it ought to print. The first version of these
animations was invented from the source, and checking it against a running
binary found differences in most of them: syn stamps its log lines with the
time and the level, the dry-run format was nothing like what was drawn, and
half the wordings were close but not right. A picture of a terminal that
does not match the terminal is worse than no picture.

**The layout is computed.** SVG collapses runs of spaces, so output drawn by
hand drifts left of where it belongs, and a line placed by eye is a pixel out
somewhere. Lines are laid on a fixed grid here and the spaces that matter are
escaped, so the columns line up by construction.

    python docs/terminals.py

Colour: syn itself prints plain text - the levels are not coloured on the way
out. Errors are shown red here because they really do go to stderr, and the
prompt is tinted to separate what you type from what comes back. Nothing else
is recoloured.
"""

import os

OUT = os.path.dirname(os.path.abspath(__file__))

FG     = "#c9d1d9"   # what a terminal prints in
DIM    = "#8b949e"   # comments, and the parts of a line that are furniture
PROMPT = "#a371f7"
ERR    = "#f85149"   # stderr
WARN   = "#d29922"

FONT = ("ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, "
        "'Liberation Mono', monospace")

LINE_H = 19
TOP    = 62
LEFT   = 22
SIZE   = 13

WEIGHT = {"cmd": 2.4, "cont": 0.3, "blank": 0.5}


def esc(s):
    return (s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
             .replace(" ", "\u00a0"))


def plain(s):
    return s.replace("\u00a0", " ")


# Measured from a rendered screen: Consolas at 13px advances about this far
# per character. Used only to size the canvas, never to place anything.
CHAR_W = 7.15


def longest(lines):
    widest = 0
    for line in lines:
        k = line[0]
        if k == "blank":
            continue
        text = line[2] if k == "log" else line[1]
        if k == "log":
            text = "2026-10-19 07:42:09 [ERROR] " + text
        elif k == "cmd":
            text = "$ " + text
        elif k == "note":
            text = "# " + text
        widest = max(widest, len(text.expandtabs(8)))
    return widest


def build(name, title, caption, lines, loop=16, width=None):
    # Wide enough for the longest line rather than a round number somebody
    # liked: a terminal whose output runs off the edge is not a terminal.
    # The README displays every one of these at the same width, so a wider
    # canvas simply means slightly smaller text, not a wider picture.
    if width is None:
        width = max(760, int(LEFT * 2 + longest(lines) * CHAR_W) + 8)
    """lines: (kind, text) or (kind, level, text) for log lines.

    kind: cmd | cont | out | log | note | blank
    """
    height = TOP + LINE_H * len(lines) + 24

    weights = [WEIGHT.get(l[0], 1.0) for l in lines]
    total = sum(weights) or 1
    at, acc = [], 0.0
    for w in weights:
        at.append(2 + acc / total * 84)
        acc += w

    css, body = [], []
    for i, (line, start) in enumerate(zip(lines, at)):
        kind = line[0]
        cls = f"l{i}"
        css.append(
            f"    .{cls} {{ animation: k{i} {loop}s linear infinite }}\n"
            f"    @keyframes k{i} {{ 0%,{start:.1f}%{{opacity:0}} "
            f"{start + 1:.1f}%,95%{{opacity:1}} 100%{{opacity:0}} }}"
        )
        y = TOP + LINE_H * i

        if kind == "blank":
            body.append(f'    <text class="line {cls}" x="{LEFT}" y="{y}"> </text>')

        elif kind == "cmd":
            body.append(
                f'    <text class="line {cls}" x="{LEFT}" y="{y}">'
                f'<tspan fill="{PROMPT}" font-weight="700">$</tspan>'
                f'<tspan fill="{FG}" dx="8">{esc(line[1])}</tspan></text>'
            )

        elif kind == "cont":
            body.append(f'    <text class="line {cls}" x="{LEFT}" y="{y}" '
                        f'fill="{FG}">{esc(line[1])}</text>')

        elif kind == "log":
            # exactly how syn writes it: "2026-10-19 07:42:09 [INFO] message"
            level, text = line[1], line[2]
            colour = ERR if level == "ERROR" else WARN if level == "WARN" else FG
            stamp = f"2026-10-19 07:42:{STAMP[0]:02d}"
            STAMP[0] = min(59, STAMP[0] + 3)
            body.append(
                f'    <text class="line {cls}" x="{LEFT}" y="{y}" fill="{colour}">'
                f'<tspan fill="{DIM}">{esc(stamp)} [{level}]</tspan>'
                f'<tspan dx="7">{esc(text)}</tspan></text>'
            )

        elif kind == "note":
            # a shell comment - something you would type, not something printed
            body.append(f'    <text class="line {cls}" x="{LEFT}" y="{y}" '
                        f'fill="{DIM}">{esc("# " + line[1])}</text>')

        else:  # out - stdout, printed plain
            body.append(f'    <text class="line {cls}" x="{LEFT}" y="{y}" '
                        f'fill="{FG}">{esc(line[1])}</text>')

    svg = f"""<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" width="{width}" height="{height}" font-family="{FONT}" role="img" aria-label="{plain(caption)}">
  <title>{title}</title>

  <style>
    .bg     {{ fill: #0d1117 }}
    .chrome {{ fill: #161b22 }}
    .edge   {{ fill: none; stroke: #30363d; stroke-width: 1 }}
    text    {{ font-size: {SIZE}px }}
    /* Visible by default; the keyframes do the hiding, so a renderer that
       does not animate shows the finished screen and not an empty box. */
    .line   {{ opacity: 1 }}
{chr(10).join(css)}
    @media (prefers-reduced-motion: reduce) {{
      .line {{ animation: none; opacity: 1 }}
    }}
  </style>

  <rect class="bg" width="{width}" height="{height}" rx="10"/>
  <rect class="chrome" width="{width}" height="32" rx="10"/>
  <rect class="chrome" y="22" width="{width}" height="10"/>
  <rect class="edge" x="0.5" y="0.5" width="{width - 1}" height="{height - 1}" rx="10"/>

  <circle cx="19" cy="16" r="5" fill="#ff5f57"/>
  <circle cx="37" cy="16" r="5" fill="#febc2e"/>
  <circle cx="55" cy="16" r="5" fill="#28c840"/>
  <text x="{width // 2}" y="21" text-anchor="middle" fill="{DIM}" font-size="12">{plain(title)}</text>

  <g>
{chr(10).join(body)}
  </g>
</svg>
"""
    with open(os.path.join(OUT, name), "w", encoding="utf-8", newline="\n") as f:
        f.write(svg)
    return height


STAMP = [9]   # the seconds counter, so timestamps advance down a screen

L = lambda lvl, text: ("log", lvl, text)
C = lambda text: ("cmd", text)
O = lambda text: ("out", text)
N = lambda text: ("note", text)
B = ("blank", "")


SCREENS = [
    ("demo.svg", "syn — two machines, one history",
     "Switching syn on, asking what it is doing, then a push arriving and the bot restarting", [
        C("syn auto on"),
        O("  login      on    sync once when you log in"),
        O("  watch      on    then keep syncing every 10 minutes"),
        O("  shutdown   on    publish before the machine goes down, and hold it until done"),
        B,
        C("syn status my-notes"),
        O("  watcher:  running (pid 26676), started 2 h ago, checked 1 min ago"),
        O("            next look at my-notes in 9 min"),
        B,
        O("my-notes  (C:\\Users\\me\\my-notes)"),
        O("  branch:   main -> origin/main"),
        O("  local:    clean"),
        O("  vs remote: 0 ahead, 0 behind"),
        O("  last sync: 2026-10-19 07:42:37 (just now)"),
        B,
        N("todo.md pushed from the laptop; the watcher picks it up"),
        L("INFO", "my-notes: pulled 1 commit from origin/main"),
        O("           down  a91f3c2  notes from the laptop"),
        O("                 modified  todo.md"),
     ]),

    ("t-quickstart.svg", "the first five minutes",
     "Registering a repository and switching automatic syncing on", [
        C("syn auth login --pat ghp_..."),
        L("INFO", "logged in as FalconTechDev"),
        B,
        C("syn init"),
        L("INFO", "registered \"my-notes\" -> origin (https://github.com/me/my-notes.git)"),
        L("INFO", "next: `syn auto on` to keep it in sync by itself"),
        B,
        C("syn auto on"),
        O("  login      on    sync once when you log in"),
        O("  watch      on    then keep syncing every 10 minutes"),
        O("  shutdown   on    publish before the machine goes down, and hold it until done"),
        B,
        O("  Repositories"),
        O("    my-notes   every 10 minutes"),
     ]),

    ("t-repos.svg", "tracking repositories",
     "Registering a clone you have, listing what is tracked, and removing one", [
        C("syn init"),
        L("INFO", "registered \"my-notes\" -> origin (https://github.com/me/my-notes.git)"),
        L("INFO", "next: `syn auto on` to keep it in sync by itself"),
        B,
        C("syn ls"),
        O("  my-notes  every 10 minutes  origin/(current branch)"),
        O("            C:\\Users\\me\\my-notes"),
        O("  my-bot    every 1 minute    origin/(current branch)"),
        O("            /srv/my-bot"),
        B,
        O("  Name one of these, or \"all\", when you pull, push or sync."),
        B,
        C("syn remove my-notes"),
        L("INFO", "stopped tracking \"my-notes\" (files untouched)"),
     ]),

    ("t-sync.svg", "syncing by hand",
     "A dry run first, then the real sync, then checking nothing is left", [
        C("syn sync my-notes --dry-run"),
        O("my-notes"),
        O("  would commit 2 files:"),
        O("      modified  todo.md"),
        O("      new       ideas.md"),
        B,
        C("syn sync my-notes"),
        L("INFO", "my-notes: committed local changes, pushed 1 commit to origin/main"),
        O("           up    dcde8ff  sync: auto-commit from desktop at 2026-10-19 07:42"),
        O("                 modified  todo.md"),
        O("                 new       ideas.md"),
        B,
        C("syn diff my-notes"),
        O("(nothing uncommitted)"),
     ]),

    ("t-auto.svg", "automatic syncing",
     "The three switches, the repositories they cover, and a pause", [
        C("syn auto"),
        O("  login      on    sync once when you log in"),
        O("  watch      on    then keep syncing every 10 minutes"),
        O("  shutdown   on    publish before the machine goes down, and hold it until done"),
        B,
        O("  Repositories"),
        O("    my-notes   every 10 minutes"),
        O("    my-bot     every 1 minute"),
        B,
        O("  All three:      syn auto on|off"),
        O("  One at a time:  syn auto login|watch|shutdown on|off"),
        O("  How often:      syn auto watch 30m"),
        O("  One repo:       syn auto <repo> on|off"),
        B,
        C("syn pause my-bot 2h"),
        L("INFO", "my-bot: left out of automatic syncing until 09:42 on 19 Oct"),
        O("  Commands you type yourself still work as normal."),
     ]),

    ("t-files.svg", "part of a repository",
     "Syncing only one folder, and keeping a local file out of automatic commits", [
        C("syn files include big-repo \"src/**\""),
        L("INFO", "big-repo: include pattern \"src/**\" added"),
        B,
        C("syn files big-repo"),
        O("big-repo:"),
        O("  include:"),
        O("    src/**"),
        O("  exclude:"),
        B,
        O("  active sparse-checkout:"),
        O("  src/**"),
        B,
        C("syn ignore my-bot \"*.env\""),
        L("INFO", "my-bot: *.env held back from automatic commits"),
        B,
        C("syn files clear big-repo"),
        L("INFO", "big-repo: all file rules cleared (all files will be synced)"),
     ]),

    ("t-conflict.svg", "a conflict, start to finish",
     "A sync stops on a conflict, the next one refuses, and resolve finishes it", [
        C("syn sync my-notes"),
        L("ERROR", "my-notes: conflict on main (your uncommitted edits conflict with the pulled changes)"),
        L("ERROR", "1 repository needs attention - see `syn status`"),
        B,
        C("syn sync my-notes"),
        L("ERROR", "my-notes: FAILED - a conflict from 2026-10-19 07:43 is still open, so syncing is on hold"),
        O("        files: todo.md"),
        O("        edit them, then:  syn resolve my-notes"),
        O("        or pick a side:   syn resolve my-notes --ours | --theirs"),
        O("        your work before the conflict is on syn/backup/desktop/main/20261019-074334"),
        B,
        N("you edit todo.md and delete the <<<<<<< ======= >>>>>>> lines"),
        B,
        C("syn resolve my-notes"),
        L("INFO", "my-notes: markers are gone from todo.md"),
        L("INFO", "my-notes: committed local changes, pushed 1 commit to origin/main"),
     ]),

    ("t-hooks.svg", "hooks",
     "A hook that only fires for the files it cares about, and what hook ls shows", [
        C("syn hook add my-bot after-pull --paths \"src/**,go.mod\" \\"),
        ("cont", "      \"systemctl restart my-bot\""),
        L("INFO", "my-bot: after-pull hook added, only for src/**, go.mod"),
        B,
        C("syn hook rollback my-bot on"),
        L("INFO", "my-bot: a failing after-pull hook will put the repository back"),
        B,
        C("syn hook ls"),
        O("my-bot  (timeout 300s, rolls back a pull if the hook fails)"),
        O("  after-pull   systemctl restart my-bot"),
        O("                 only when these change: src/**, go.mod"),
        B,
        N("the command lives here, never in git - a push cannot change it"),
     ]),

    ("t-server.svg", "a server, set up once",
     "Deploy target, CI gate, and the program syn keeps running", [
        C("syn deploy my-bot on"),
        L("INFO", "my-bot: deploy target - syncing brings changes down and sends nothing up"),
        L("INFO", "       anything written there stays on this machine, uncommitted"),
        B,
        C("syn set my-bot require_checks on"),
        L("INFO", "my-bot.require_checks = \"on\""),
        B,
        C("syn app my-bot -- python bot.py"),
        L("INFO", "my-bot: runs python bot.py"),
        L("INFO", "       restarted whenever an update arrives"),
        L("INFO", "       the watcher starts it:  syn watch my-bot"),
        L("INFO", "       or try it here first:   syn app my-bot start"),
        B,
        C("sudo syn service install --as deploy"),
        L("INFO", "wrote /etc/systemd/system/synchronize.service"),
        L("INFO", "synchronize.service is running as deploy, and will come back after a reboot"),
     ]),

    ("t-scripting.svg", "for scripts",
     "The porcelain form, JSON output, and an exit code a script can act on", [
        C("syn status all --porcelain"),
        O("my-notes\tok\t0\t0\t0\tmain\tC:\\Users\\me\\my-notes"),
        B,
        C("syn status all --json"),
        O("["),
        O("  {"),
        O("    \"name\": \"my-notes\","),
        O("    \"path\": \"C:\\\\Users\\\\me\\\\my-notes\","),
        O("    \"branch\": \"main\","),
        O("    \"tracking\": \"origin/main\","),
        O("    \"ahead\": 0,"),
        O("    \"behind\": 0,"),
        O("    \"dirty\": false,"),
        O("    \"changed\": 0,"),
        O("    \"auto\": true,"),
        O("    \"last_sync\": \"2026-10-19T04:42:37Z\""),
        O("  }"),
        O("]"),
        B,
        N("ahead or behind is not a failure; a conflict or an unreadable repo is"),
     ]),

    ("t-alerts.svg", "alerts",
     "Setting up Telegram alerts and sending a test one", [
        C("syn config telegram 8012345678:AAE... 123456789"),
        L("INFO", "Telegram alerts will go to chat 123456789"),
        L("INFO", "       check it works:  syn config test"),
        B,
        C("syn config telegram"),
        O("Telegram alerts go to chat 123456789"),
        O("  sending on:  conflict"),
        O("  test it:     syn config test"),
        O("  switch off:  syn config telegram off"),
        B,
        C("syn config test"),
        L("INFO", "sending a test alert..."),
        B,
        O("  Sent. A desktop notification should have appeared just now."),
        O("  Telegram: check chat 123456789."),
        O("  If nothing arrived, the log says why:  syn log 20"),
     ]),

    ("t-doctor.svg", "checking on it",
     "doctor on a healthy setup, and the log narrowed to one repository", [
        C("syn doctor"),
        O("synchronize v1.3.0 on windows/amd64"),
        O("  git:        git version 2.54.0.windows.1"),
        O("  home:       C:/Users/me/.synchronize"),
        O("  repos:      2 registered"),
        O("  background: ON - syncs at login, publishes before shutdown"),
        B,
        O("  every repository looks healthy"),
        B,
        C("syn log my-notes 3"),
        L("INFO", "my-notes: pulled 1 commit from origin/main"),
        L("INFO", "my-notes: already up to date with origin/main"),
        L("INFO", "my-notes: committed local changes, pushed 1 commit to origin/main"),
     ]),
]


if __name__ == "__main__":
    for name, title, caption, lines in SCREENS:
        STAMP[0] = 9
        h = build(name, title, caption, lines)
        print(f"  {name:<22} {len(lines):>2} lines   {h:>3}px   "
              f"{os.path.getsize(os.path.join(OUT, name)) // 1024} KB")
