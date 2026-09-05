#!/usr/bin/env sh
# Installs syn on Linux or macOS. Builds from source when run inside a clone,
# otherwise installs a binary sitting next to this script.
#
#   ./install.sh              install for you only (~/.local/bin)
#   ./install.sh --system     install for everyone (/usr/local/bin, needs sudo)
#   ./install.sh --uninstall  remove it again

set -eu

name=syn
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root=$(CDPATH= cd -- "$here/../.." && pwd)

dest="$HOME/.local/bin"
action=install

for arg in "$@"; do
    case "$arg" in
        --system)    dest=/usr/local/bin ;;
        --uninstall) action=uninstall ;;
        -h|--help)
            sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'
            exit 0 ;;
        *)
            echo "unknown option: $arg" >&2
            exit 1 ;;
    esac
done

as_root() {
    if [ -w "$dest" ] || [ "$(id -u)" = 0 ]; then
        "$@"
    elif command -v sudo >/dev/null 2>&1; then
        sudo "$@"
    else
        echo "need write access to $dest, and sudo is not available" >&2
        exit 1
    fi
}

if [ "$action" = uninstall ]; then
    if [ -x "$dest/$name" ]; then
        "$dest/$name" auto off >/dev/null 2>&1 || true
        as_root rm -f "$dest/$name"
        echo "removed $dest/$name"
    else
        echo "$name is not installed in $dest"
    fi
    echo "your settings in ~/.synchronize were left alone"
    exit 0
fi

# --- find or build the binary ------------------------------------------------

if [ -x "$here/$name" ]; then
    binary="$here/$name"
elif [ -f "$root/go.mod" ]; then
    command -v go >/dev/null 2>&1 || {
        echo "go is needed to build from source - see https://go.dev/dl" >&2
        exit 1
    }
    echo "building $name..."
    ( cd "$root" && go build -ldflags="-s -w" -o "bin/$name" ./cmd/syn/ )
    binary="$root/bin/$name"
else
    echo "no $name binary here, and no source to build it from" >&2
    exit 1
fi

command -v git >/dev/null 2>&1 || {
    echo
    echo "  note: git is not installed, and syn drives the real git."
    echo "        install it with your package manager first."
    echo
}

# --- install -----------------------------------------------------------------

as_root mkdir -p "$dest"
as_root cp "$binary" "$dest/$name"
as_root chmod 755 "$dest/$name"
echo "installed $dest/$name"

case ":$PATH:" in
    *":$dest:"*) ;;
    *)
        echo
        echo "  $dest is not on your PATH. Add this to your shell profile:"
        echo
        echo "      export PATH=\"\$PATH:$dest\""
        echo ;;
esac

echo
echo "Next:"
echo "  syn auth login           connect your GitHub account"
echo "  syn repo add owner/name  clone it and start tracking"
echo "  syn auto on              sync at login, on a timer, and before shutdown"
