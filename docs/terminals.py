#!/usr/bin/env python3
"""Builds the terminal animations the README uses.

Every one of these is a picture of the tool being used, and the one thing a
picture of a terminal has to get right is that the columns line up. Hand-
writing the SVG got that wrong twice: SVG collapses runs of spaces, so the
output drifted left, and a line placed by eye is a line that is one pixel off
somewhere. So the lines are laid out here on a fixed grid instead, and the
file is regenerated rather than edited:

    python docs/terminals.py

The animation reveals the lines in order, as a terminal does. Each line is
visible by default and the keyframes hide it, so a renderer that does not
animate - an image proxy, a PDF, a reader with motion turned down - shows the
finished screen rather than an empty black box.
"""

import os
import re

OUT = os.path.dirname(os.path.abspath(__file__))

FG     = "#c9d1d9"
DIM    = "#8b949e"
PROMPT = "#a371f7"
OK     = "#3fb950"
INFO   = "#58a6ff"
WARN   = "#d29922"
ERR    = "#f85149"

COLOURS = {
    "cmd": FG, "out": FG, "dim": DIM, "ok": OK,
    "info": INFO, "warn": WARN, "err": ERR, "blank": DIM, "cont": FG,
}

FONT = ("ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, "
        "'Liberation Mono', monospace")

LINE_H = 19          # distance between baselines
TOP    = 62          # first baseline, below the title bar
LEFT   = 22
SIZE   = 13.5

# How long a line lingers before the next appears. A command gets more,
# because that is the beat where a person would be typing it.
WEIGHT = {"cmd": 2.4, "cont": 0.3, "blank": 0.5}


def esc(s):
    return (s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
             .replace(" ", "\u00a0"))


def build(name, title, caption, lines, loop=16, width=760):
    """lines: list of (kind, text). kind: cmd | cont | out | dim | ok | info | warn | err | blank"""
    height = TOP + LINE_H * len(lines) + 24

    weights = [WEIGHT.get(k, 1.0) for k, _ in lines]
    total = sum(weights) or 1
    # Lines appear across the first 86% of the loop; the rest is a hold on
    # the finished screen, so it can actually be read.
    at, acc = [], 0.0
    for w in weights:
        at.append(2 + acc / total * 84)
        acc += w

    css, body = [], []
    for i, ((kind, text), start) in enumerate(zip(lines, at)):
        cls = f"l{i}"
        css.append(
            f"    .{cls} {{ animation: k{i} {loop}s linear infinite }}\n"
            f"    @keyframes k{i} {{ 0%,{start:.1f}%{{opacity:0}} "
            f"{start + 1:.1f}%,95%{{opacity:1}} 100%{{opacity:0}} }}"
        )
        y = TOP + LINE_H * i
        if kind == "blank":
            body.append(f'    <text class="line {cls}" x="{LEFT}" y="{y}"> </text>')
        elif kind == "cont":
            # A command wrapped onto a second line: no prompt, because a
            # prompt there would read as a second command.
            body.append(
                f'    <text class="line {cls}" x="{LEFT}" y="{y}" '
                f'fill="{FG}">{esc(text)}</text>'
            )
        elif kind == "cmd":
            body.append(
                f'    <text class="line {cls}" x="{LEFT}" y="{y}">'
                f'<tspan fill="{PROMPT}" font-weight="700">$</tspan>'
                f'<tspan fill="{FG}" dx="8">{esc(text)}</tspan></text>'
            )
        else:
            body.append(
                f'    <text class="line {cls}" x="{LEFT}" y="{y}" '
                f'fill="{COLOURS[kind]}">{esc(text)}</text>'
            )

    svg = f"""<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" width="{width}" height="{height}" font-family="{FONT}" role="img" aria-label="{esc(caption).replace(chr(160), ' ')}">
  <title>{title}</title>

  <style>
    .bg     {{ fill: #0d1117 }}
    .chrome {{ fill: #161b22 }}
    .edge   {{ fill: none; stroke: #30363d; stroke-width: 1 }}
    text    {{ font-size: {SIZE}px }}
    /* Visible by default; the keyframes do the hiding. A renderer that does
       not animate then shows the finished screen, not an empty box. */
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
  <text x="{width // 2}" y="21" text-anchor="middle" fill="{DIM}" font-size="12">{esc(title).replace(chr(160), ' ')}</text>

  <g>
{chr(10).join(body)}
  </g>
</svg>
"""
    path = os.path.join(OUT, name)
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write(svg)
    return path, height


# --------------------------------------------------------------------------
# The screens. Each is a real session: these are the commands as they are,
# and the output as the tool actually prints it.
# --------------------------------------------------------------------------

SCREENS = [
    ("demo.svg", "syn — two machines, one history",
     "Switching syn on, asking what it is doing, then a push arriving and the bot restarting", [
        ("cmd",  "syn auto on"),
        ("ok",   "  login     on   sync once when you log in"),
        ("ok",   "  watch     on   then keep syncing every 10 minutes"),
        ("ok",   "  shutdown  on   publish before the machine goes down"),
        ("blank", ""),
        ("cmd",  "syn status all"),
        ("info", "  watcher:  running (pid 26676), checked 1 min ago"),
        ("dim",  "            next look at my-bot in 9 min"),
        ("blank", ""),
        ("out",  "my-bot  (/srv/my-bot)"),
        ("dim",  "  branch:    main -> origin/main"),
        ("ok",   "  local:     clean"),
        ("dim",  "  vs remote: 0 ahead, 0 behind"),
        ("dim",  "  last sync: 1 min ago"),
        ("blank", ""),
        ("dim",  "... bot.py pushed from the laptop ..."),
        ("info", "my-bot: pulled 1 commit from origin/main"),
        ("dim",  "                 modified  bot.py"),
        ("info", "my-bot: the code changed - restarting python bot.py"),
        ("ok",   "my-bot | bot v2 running"),
     ]),

    ("t-quickstart.svg", "the first five minutes",
     "Signing in, adding a repository, switching automatic syncing on", [
        ("cmd",  "syn auth login --pat ghp_..."),
        ("ok",   "  logged in as Falcon (FalconTechDev)"),
        ("blank", ""),
        ("cmd",  "syn repo add me/my-notes"),
        ("dim",  "  cloning into C:\\Users\\me\\my-notes ..."),
        ("ok",   "  my-notes: tracking origin/main, every 10 minutes"),
        ("blank", ""),
        ("cmd",  "syn auto on"),
        ("ok",   "  login     on   sync once when you log in"),
        ("ok",   "  watch     on   then keep syncing every 10 minutes"),
        ("ok",   "  shutdown  on   publish before the machine goes down"),
        ("blank", ""),
        ("dim",  "that is a complete setup - nothing else is required"),
     ]),

    ("t-repos.svg", "tracking repositories",
     "Registering a clone you have, cloning a new one, listing and removing", [
        ("cmd",  "syn init"),
        ("ok",   "  registered \"notes\" -> origin (github.com/me/notes)"),
        ("blank", ""),
        ("cmd",  "syn repo add me/my-bot --path /srv/my-bot"),
        ("ok",   "  my-bot: tracking origin/main"),
        ("blank", ""),
        ("cmd",  "syn repo add https://gitlab.com/group/thing.git"),
        ("dim",  "  any git remote works - git handles the credentials"),
        ("blank", ""),
        ("cmd",  "syn ls"),
        ("out",  "  notes     every 10 minutes   origin/main"),
        ("dim",  "            C:\\Users\\me\\notes"),
        ("out",  "  my-bot    every 1 minute     origin/main"),
        ("dim",  "            /srv/my-bot"),
        ("blank", ""),
        ("cmd",  "syn remove notes"),
        ("dim",  "  stopped tracking \"notes\" (files untouched)"),
     ]),

    ("t-sync.svg", "syncing by hand",
     "pull, push and sync, with a dry run first", [
        ("cmd",  "syn sync my-notes --dry-run"),
        ("dim",  "my-notes"),
        ("dim",  "  would pull 2 commits from origin/main"),
        ("dim",  "  would commit 3 files and push them"),
        ("blank", ""),
        ("cmd",  "syn sync my-notes"),
        ("info", "my-notes: pulled 2 commits from origin/main"),
        ("dim",  "           down  a91f3c2  notes from the laptop"),
        ("dim",  "                 modified  todo.md"),
        ("info", "my-notes: committed local changes, pushed 1 commit"),
        ("dim",  "                 modified  journal.md"),
        ("dim",  "                 new       ideas.md"),
        ("blank", ""),
        ("cmd",  "syn diff my-notes"),
        ("ok",   "  nothing uncommitted - everything is published"),
     ]),

    ("t-auto.svg", "automatic syncing",
     "The three switches, a per-repository exception, and a pause", [
        ("cmd",  "syn auto"),
        ("ok",   "  login     on    sync once when you log in"),
        ("ok",   "  watch     on    then keep syncing every 10 minutes"),
        ("ok",   "  shutdown  on    publish before the machine goes down"),
        ("blank", ""),
        ("cmd",  "syn auto watch 30m"),
        ("ok",   "  checked every 30 minutes from now on"),
        ("blank", ""),
        ("cmd",  "syn auto big-media off"),
        ("dim",  "  big-media: left out of automatic syncing"),
        ("dim",  "             commands you type still reach it"),
        ("blank", ""),
        ("cmd",  "syn pause my-bot 2h"),
        ("warn", "  my-bot: paused until 16:42"),
        ("dim",  "          the running watcher notices without a restart"),
     ]),

    ("t-files.svg", "part of a repository",
     "Syncing only one folder, and keeping a local file out of commits", [
        ("cmd",  "syn files include big-repo \"src/**\""),
        ("ok",   "  big-repo: include pattern \"src/**\" added"),
        ("dim",  "            everything else left the folder, and stays"),
        ("dim",  "            on the remote - not committed as deleted"),
        ("blank", ""),
        ("cmd",  "syn files big-repo"),
        ("out",  "  active sparse-checkout:"),
        ("info", "  src/**"),
        ("blank", ""),
        ("cmd",  "syn ignore my-bot \"*.env\""),
        ("ok",   "  my-bot: *.env stays on disk, out of automatic commits"),
        ("blank", ""),
        ("cmd",  "syn files clear big-repo"),
        ("dim",  "  big-repo: the whole repository again"),
     ]),

    ("t-conflict.svg", "a conflict, start to finish",
     "A sync stops on a conflict; you edit the files; resolve finishes it", [
        ("cmd",  "syn sync my-notes"),
        ("err",  "my-notes: CONFLICT on main (todo.md)"),
        ("dim",  "          backup branch syn/backup/desktop/main/20261019-1442"),
        ("warn", "          syncing is on hold until you say what to do"),
        ("blank", ""),
        ("cmd",  "syn sync my-notes"),
        ("err",  "my-notes: FAILED - a conflict from 14:42 is still open"),
        ("dim",  "          files: todo.md"),
        ("dim",  "          edit them, then:  syn resolve my-notes"),
        ("blank", ""),
        ("dim",  "... you edit todo.md and delete the <<<<<<< lines ..."),
        ("blank", ""),
        ("cmd",  "syn resolve my-notes"),
        ("ok",   "my-notes: markers are gone from todo.md"),
        ("info", "my-notes: pushed 1 commit to origin/main"),
     ]),

    ("t-hooks.svg", "hooks",
     "A hook that only fires for the files it cares about, and what it is told", [
        ("cmd",  "syn hook add my-bot after-pull --paths \"src/**,go.mod\" \\"),
        ("cont", "      \"systemctl restart my-bot\""),
        ("ok",   "  my-bot: after-pull hook added, only for src/**, go.mod"),
        ("blank", ""),
        ("cmd",  "syn hook ls"),
        ("out",  "my-bot  (timeout 300s, rolls back a pull if the hook fails)"),
        ("dim",  "  after-pull   systemctl restart my-bot"),
        ("dim",  "                 only when these change: src/**, go.mod"),
        ("blank", ""),
        ("dim",  "the hook is told what happened:"),
        ("info", "  SYN_EVENT=after-pull   SYN_REPO=my-bot"),
        ("info", "  SYN_COMMIT=9f31a0c     SYN_PREV_COMMIT=4ab77e1"),
        ("info", "  SYN_CHANGED_FILES      one path per line"),
        ("info", "  SYN_CHANGED_COUNT=2"),
        ("blank", ""),
        ("dim",  "commands live on this machine and never travel through git"),
     ]),

    ("t-server.svg", "a server, set up once",
     "Deploy target, CI gate, the program syn keeps running, and the boot service", [
        ("cmd",  "syn repo add me/my-bot --path /srv/my-bot"),
        ("ok",   "  my-bot: tracking origin/main"),
        ("blank", ""),
        ("cmd",  "syn deploy my-bot on"),
        ("ok",   "  my-bot: deploy target - changes come down, nothing goes up"),
        ("dim",  "          logs and caches it writes stay on this machine"),
        ("blank", ""),
        ("cmd",  "syn set my-bot require_checks on"),
        ("ok",   "  my-bot.require_checks = \"on\""),
        ("blank", ""),
        ("cmd",  "syn app my-bot -- python bot.py"),
        ("ok",   "  my-bot: runs python bot.py"),
        ("dim",  "          restarted whenever an update arrives"),
        ("blank", ""),
        ("cmd",  "sudo syn service install --as deploy"),
        ("ok",   "  synchronize.service is running as deploy"),
        ("dim",  "  watch it with:  journalctl -u synchronize -f"),
     ]),

    ("t-scripting.svg", "for scripts",
     "JSON output, the porcelain form, and exit codes that mean something", [
        ("cmd",  "syn status all --porcelain"),
        ("out",  "my-notes  ok        0  0  0  main  /home/me/notes"),
        ("out",  "my-bot    conflict  0  2  1  main  /srv/my-bot"),
        ("blank", ""),
        ("cmd",  "syn status all --json | jq '.[] | select(.conflict)'"),
        ("out",  "{"),
        ("out",  "  \"name\": \"my-bot\","),
        ("out",  "  \"branch\": \"main\","),
        ("out",  "  \"behind\": 2,"),
        ("out",  "  \"conflict\": \"local commits conflict with the remote\""),
        ("out",  "}"),
        ("blank", ""),
        ("cmd",  "syn status all > state.json || notify \"syn needs attention\""),
        ("err",  "  my-bot has a conflict open - run `syn resolve`"),
        ("warn", "  exit 1"),
        ("blank", ""),
        ("dim",  "ahead or behind is not a failure - that is the normal state"),
     ]),

    ("t-alerts.svg", "alerts",
     "Setting up Telegram alerts and checking they arrive", [
        ("cmd",  "syn config telegram 8012345678:AAE... 123456789"),
        ("ok",   "  Telegram alerts will go to chat 123456789"),
        ("dim",  "         check it works:  syn config test"),
        ("blank", ""),
        ("cmd",  "syn config telegram"),
        ("out",  "Telegram alerts go to chat 123456789"),
        ("dim",  "  sending on:  conflict"),
        ("dim",  "  switch off:  syn config telegram off"),
        ("blank", ""),
        ("cmd",  "syn config test"),
        ("info", "  sending a test alert..."),
        ("ok",   "  Sent. A desktop notification should have appeared."),
        ("dim",  "  Telegram: check chat 123456789."),
        ("blank", ""),
        ("dim",  "the token is a password for the bot - it is never printed back"),
     ]),

    ("t-doctor.svg", "checking on it",
     "doctor, and the log narrowed to one repository", [
        ("cmd",  "syn doctor"),
        ("out",  "synchronize v1.3.0 on windows/amd64"),
        ("dim",  "  git:        git version 2.54.0"),
        ("dim",  "  home:       C:\\Users\\me\\.synchronize"),
        ("dim",  "  repos:      4 registered"),
        ("ok",   "  background: ON - syncs at login, publishes before shutdown"),
        ("blank", ""),
        ("err",  "  1 repository needs attention:"),
        ("err",  "my-bot: /srv/my-bot no longer exists"),
        ("dim",  "        it may be on a drive that is not plugged in."),
        ("dim",  "        if it is gone for good:  syn remove my-bot"),
        ("warn", "  exit 1"),
        ("blank", ""),
        ("cmd",  "syn log my-notes 4"),
        ("dim",  "14:31:02 [INFO] my-notes: pulled 1 commit from origin/main"),
        ("dim",  "14:41:07 [INFO] my-notes: already up to date"),
     ]),
]


if __name__ == "__main__":
    for name, title, caption, lines in SCREENS:
        path, height = build(name, title, caption, lines)
        print(f"  {os.path.basename(path):<22} {len(lines):>2} lines   {height:>3}px   "
              f"{os.path.getsize(path) // 1024} KB")
