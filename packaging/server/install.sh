#!/usr/bin/env sh
# Installs syn on a Linux server. Run beside the syn binary:
#
#   tar xzf syn-server-linux-amd64.tar.gz
#   sudo ./install.sh
#   sudo ./install.sh --service --as deploy   # also enable the systemd unit
#
# Needs: git 2.31+, a user account that will own the repositories.

set -eu

name=syn
dest=/usr/local/bin
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
service_user=""
install_service=0

usage() {
    sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'
    echo
    echo "Options:"
    echo "  --service --as <user>   install and start the systemd unit as <user>"
    echo "  --uninstall             remove the binary and stop the service"
    exit "${1:-0}"
}

for arg in "$@"; do
    case "$arg" in
        --service)   install_service=1 ;;
        --as)        : ;;  # value handled below
        --uninstall) action=uninstall ;;
        -h|--help)   usage 0 ;;
        *)
            if [ "${prev:-}" = --as ]; then
                service_user=$arg
            else
                echo "unknown option: $arg" >&2
                usage 1
            fi ;;
    esac
    prev=$arg
done

as_root() {
    if [ "$(id -u)" = 0 ]; then
        "$@"
    elif command -v sudo >/dev/null 2>&1; then
        sudo "$@"
    else
        echo "this script needs root (try: sudo $0)" >&2
        exit 1
    fi
}

if [ "${action:-}" = uninstall ]; then
    as_root systemctl disable --now synchronize.service 2>/dev/null || true
    as_root rm -f /etc/systemd/system/synchronize.service
    as_root systemctl daemon-reload 2>/dev/null || true
    as_root rm -f "$dest/$name"
    echo "removed $dest/$name and synchronize.service"
    echo "settings in ~/.synchronize were left alone"
    exit 0
fi

binary=""
for candidate in "$here/$name" "$here/syn-linux-amd64" "$here/syn-linux-arm64"; do
    if [ -x "$candidate" ] || [ -f "$candidate" ]; then
        binary=$candidate
        break
    fi
done
if [ -z "$binary" ]; then
    echo "no syn binary found next to install.sh" >&2
    exit 1
fi

if ! command -v git >/dev/null 2>&1; then
    echo "git is required. Install it first, for example:"
    echo "  apt install git        # Debian/Ubuntu"
    echo "  dnf install git        # Fedora/RHEL"
    exit 1
fi

as_root install -m 755 "$binary" "$dest/$name"
echo "installed $dest/$name"

if [ "$install_service" = 1 ]; then
    if [ -z "$service_user" ]; then
        echo "--service needs --as <user> (the account that owns the repositories)" >&2
        exit 1
    fi
    as_root "$dest/$name" service install --as "$service_user"
fi

cat <<'EOF'

Next steps (run as the deploy user, not root):

  syn auth login --pat <token>              # GitHub token with repo scope
  syn repo add owner/name --path /srv/app   # clone and register
  syn deploy name on                        # server only pulls, never pushes
  syn set name interval 1m                  # how often to check
  syn restart name bot.py --systemd my-bot  # restart when bot.py changes

If you did not pass --service above, enable the watcher from boot with:

  sudo syn service install --as <user>

Watch logs:

  journalctl -u synchronize -f

After adding a repository, run `sudo syn service install --as <user>` again
so the service may write to the new folder.

EOF
