#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")/../.."

APP_NAME="tunnerse"
VERSION_FILE="internal/version/VERSION"
if [ -n "${TUNNERSE_VERSION:-}" ]; then
    VERSION="$TUNNERSE_VERSION"
elif [ -f "$VERSION_FILE" ]; then
    VERSION="$(tr -d '[:space:]' < "$VERSION_FILE")"
else
    VERSION="dev"
fi
VERSION_LDFLAGS="-X github.com/pedroborgesdev/tunnerse-cli/internal/version.Version=$VERSION"
DIST_DIR="dist"
WORK_ROOT="${TUNNERSE_BUILD_ROOT:-}"
CLEAN_WORK_ROOT=false

if [ -z "$WORK_ROOT" ]; then
    WORK_ROOT="$(mktemp -d)"
    CLEAN_WORK_ROOT=true
else
    mkdir -p "$WORK_ROOT"
fi

DEB_WORK_DIR="$WORK_ROOT/deb"
SERVER_WORK_DIR="$WORK_ROOT/server"

cleanup() {
    if [ "$CLEAN_WORK_ROOT" = true ]; then
        rm -rf "$WORK_ROOT"
    fi
}
trap cleanup EXIT

if [ -n "${DEB_ARCHES:-}" ]; then
    PACKAGE_ARCHES="$DEB_ARCHES"
elif [ -n "${DEB_ARCH:-}" ]; then
    PACKAGE_ARCHES="$DEB_ARCH"
else
    PACKAGE_ARCHES="amd64 x86"
fi

SERVER_ARCHES="${SERVER_ARCHES:-amd64 x86}"

echo "Building Tunnerse Debian packages..."
echo "  Package architectures: $PACKAGE_ARCHES"
echo "  Server architectures:  $SERVER_ARCHES"
echo "  Work directory:        $WORK_ROOT"
echo

if ! command -v dpkg-deb >/dev/null 2>&1; then
    echo "Error: dpkg-deb was not found."
    echo "Install dpkg-dev/debianutils and try again."
    exit 1
fi

goarch_for_deb_arch() {
    case "$1" in
        amd64)
            echo "amd64"
            ;;
        i386|x86)
            echo "386"
            ;;
        arm64)
            echo "arm64"
            ;;
        armhf)
            echo "arm"
            ;;
        *)
            echo "Error: unsupported Debian architecture '$1'. Supported: amd64, i386/x86, arm64, armhf." >&2
            return 1
            ;;
    esac
}

normalize_deb_arch() {
    case "$1" in
        x86)
            echo "i386"
            ;;
        *)
            echo "$1"
            ;;
    esac
}

artifact_arch_label() {
    case "$1" in
        i386|x86)
            echo "x86"
            ;;
        *)
            echo "$1"
            ;;
    esac
}

build_local_package() {
    local raw_arch="$1"
    local arch
    local goarch
    local pkg_root
    local debian_dir
    local installed_size
    local artifact_arch
    local package_output

    arch="$(normalize_deb_arch "$raw_arch")"
    artifact_arch="$(artifact_arch_label "$raw_arch")"
    goarch="$(goarch_for_deb_arch "$raw_arch")"
    pkg_root="$DEB_WORK_DIR/${APP_NAME}_${VERSION}_${arch}"
    debian_dir="$pkg_root/DEBIAN"
    package_output="$DIST_DIR/tunnerse-installer_${artifact_arch}_${VERSION}.deb"

    echo "[package:$arch] Preparing package tree..."
    rm -rf "$pkg_root"
    mkdir -p "$debian_dir"
    mkdir -p "$pkg_root/usr/bin"
    mkdir -p "$pkg_root/lib/systemd/system"
    mkdir -p "$pkg_root/var/lib/tunnerse/logs"
    mkdir -p "$pkg_root/etc/profile.d"
    mkdir -p "$pkg_root/usr/share/doc/tunnerse"
    mkdir -p "$pkg_root/usr/share/icons/hicolor/1920x1920/apps"
    mkdir -p "$pkg_root/usr/share/pixmaps"

    echo "[package:$arch] Building CLI and local daemon..."
    GOOS=linux GOARCH="$goarch" go build -buildvcs=false -ldflags "$VERSION_LDFLAGS" -o "$pkg_root/usr/bin/tunnerse" ./cmd/cli
    GOOS=linux GOARCH="$goarch" go build -buildvcs=false -ldflags "$VERSION_LDFLAGS" -o "$pkg_root/usr/bin/tunnerse-daemon" ./cmd/daemon
    chmod 0755 "$pkg_root/usr/bin/tunnerse" "$pkg_root/usr/bin/tunnerse-daemon"
    install -m 0644 "assets/icons/unix/tunnerse.png" "$pkg_root/usr/share/icons/hicolor/1920x1920/apps/tunnerse.png"
    install -m 0644 "assets/icons/unix/tunnerse.png" "$pkg_root/usr/share/pixmaps/tunnerse.png"

    echo "[package:$arch] Writing systemd service..."
    cat > "$pkg_root/lib/systemd/system/tunnerse-daemon.service" <<'EOF'
[Unit]
Description=Tunnerse Daemon - Local tunnel management daemon
After=network.target

[Service]
Type=simple
User=tunnerse
Group=tunnerse
Environment=TUNNERSE_DATA_DIR=/var/lib/tunnerse
ExecStart=/usr/bin/tunnerse-daemon
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF
    chmod 0644 "$pkg_root/lib/systemd/system/tunnerse-daemon.service"

    cat > "$pkg_root/etc/profile.d/tunnerse.sh" <<'EOF'
export TUNNERSE_DATA_DIR=/var/lib/tunnerse
EOF
    chmod 0644 "$pkg_root/etc/profile.d/tunnerse.sh"

    echo "[package:$arch] Writing Debian metadata..."
    installed_size="$(du -sk "$pkg_root" | awk '{print $1}')"

    cat > "$debian_dir/control" <<EOF
Package: tunnerse
Version: $VERSION
Section: net
Priority: optional
Architecture: $arch
Maintainer: Tunnerse <support@tunnerse.com>
Installed-Size: $installed_size
Depends: systemd
Conflicts: tunnerse-cli
Replaces: tunnerse-cli
Description: Tunnerse CLI and local tunnel daemon
 Tunnerse exposes local development services through public tunnel URLs.
 This package installs the tunnerse CLI and the tunnerse-daemon systemd service.
EOF

    cat > "$debian_dir/preinst" <<'EOF'
#!/bin/sh
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl stop tunnerse-daemon.service >/dev/null 2>&1 || true
    systemctl stop tunnerse-server.service >/dev/null 2>&1 || true
fi

pkill -f "/tunnerse-daemon$" >/dev/null 2>&1 || true
pkill -f "/tunnerse-server$" >/dev/null 2>&1 || true
pkill -f "/tunnerse$" >/dev/null 2>&1 || true

exit 0
EOF

    cat > "$debian_dir/postinst" <<'EOF'
#!/bin/sh
set -e

if ! id tunnerse >/dev/null 2>&1; then
    useradd --system --home /var/lib/tunnerse --shell /usr/sbin/nologin tunnerse
fi

mkdir -p /var/lib/tunnerse/logs
chown -R tunnerse:tunnerse /var/lib/tunnerse

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl disable tunnerse-server.service >/dev/null 2>&1 || true
    systemctl enable tunnerse-daemon.service || true
    systemctl restart tunnerse-daemon.service || true
fi

exit 0
EOF

    cat > "$debian_dir/prerm" <<'EOF'
#!/bin/sh
set -e

if [ "$1" = "remove" ] || [ "$1" = "deconfigure" ]; then
    if command -v systemctl >/dev/null 2>&1; then
        systemctl stop tunnerse-daemon.service || true
        systemctl disable tunnerse-daemon.service || true
    fi
fi

exit 0
EOF

    cat > "$debian_dir/postrm" <<'EOF'
#!/bin/sh
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
fi

if [ "$1" = "purge" ]; then
    rm -rf /var/lib/tunnerse
    if id tunnerse >/dev/null 2>&1; then
        userdel tunnerse || true
    fi
fi

exit 0
EOF

    chmod 0755 "$debian_dir/preinst" "$debian_dir/postinst" "$debian_dir/prerm" "$debian_dir/postrm"

    cat > "$pkg_root/usr/share/doc/tunnerse/README.Debian" <<'EOF'
Tunnerse Debian package
-----------------------

This package installs:

  /usr/bin/tunnerse
  /usr/bin/tunnerse-daemon
  /lib/systemd/system/tunnerse-daemon.service

The service runs as the system user "tunnerse" and stores logs in:

  /var/lib/tunnerse/logs

Useful commands:

  sudo systemctl status tunnerse-daemon
  sudo systemctl restart tunnerse-daemon
  sudo journalctl -u tunnerse-daemon -f
EOF
    gzip -n -9 "$pkg_root/usr/share/doc/tunnerse/README.Debian"

    echo "[package:$arch] Building .deb..."
    mkdir -p "$DIST_DIR"
    dpkg-deb --build --root-owner-group "$pkg_root" "$package_output"
}

build_server_package() {
    local raw_arch="$1"
    local arch
    local artifact_arch
    local goarch
    local pkg_root
    local debian_dir
    local installed_size
    local package_output

    arch="$(normalize_deb_arch "$raw_arch")"
    artifact_arch="$(artifact_arch_label "$raw_arch")"
    goarch="$(goarch_for_deb_arch "$raw_arch")"
    pkg_root="$SERVER_WORK_DIR/tunnerse-server_${VERSION}_${arch}"
    debian_dir="$pkg_root/DEBIAN"
    package_output="$DIST_DIR/tunnerse-server-installer_${artifact_arch}_${VERSION}.deb"

    echo "[server:$arch] Preparing package tree..."
    rm -rf "$pkg_root"
    mkdir -p "$debian_dir"
    mkdir -p "$pkg_root/usr/bin"
    mkdir -p "$pkg_root/lib/systemd/system"
    mkdir -p "$pkg_root/etc/tunnerse"
    mkdir -p "$pkg_root/usr/share/doc/tunnerse-server"

    echo "[server:$arch] Building public server..."
    GOOS=linux GOARCH="$goarch" go build -buildvcs=false -ldflags "$VERSION_LDFLAGS" -o "$pkg_root/usr/bin/tunnerse-server" ./cmd/server
    chmod 0755 "$pkg_root/usr/bin/tunnerse-server"

    if [ -f "tunnerse.config.example" ]; then
        install -m 0644 tunnerse.config.example "$pkg_root/etc/tunnerse/tunnerse.config.example"
    fi

    cat > "$pkg_root/lib/systemd/system/tunnerse-server.service" <<'EOF'
[Unit]
Description=Tunnerse Server - Public tunnel server
After=network.target

[Service]
Type=simple
WorkingDirectory=/etc/tunnerse
ExecStart=/usr/bin/tunnerse-server
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
    chmod 0644 "$pkg_root/lib/systemd/system/tunnerse-server.service"

    installed_size="$(du -sk "$pkg_root" | awk '{print $1}')"

    cat > "$debian_dir/control" <<EOF
Package: tunnerse-server
Version: $VERSION
Section: net
Priority: optional
Architecture: $arch
Maintainer: Tunnerse <support@tunnerse.com>
Installed-Size: $installed_size
Depends: systemd
Conflicts: tunnerse-api
Replaces: tunnerse-api
Description: Tunnerse public tunnel server
 Tunnerse Server receives public tunnel traffic and coordinates request
 delivery between public clients and connected local daemons.
EOF

    cat > "$debian_dir/preinst" <<'EOF'
#!/bin/sh
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl stop tunnerse-server.service >/dev/null 2>&1 || true
fi

pkill -f "/tunnerse-server$" >/dev/null 2>&1 || true

exit 0
EOF

    cat > "$debian_dir/postinst" <<'EOF'
#!/bin/sh
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
fi

exit 0
EOF

    cat > "$debian_dir/prerm" <<'EOF'
#!/bin/sh
set -e

if [ "$1" = "remove" ] || [ "$1" = "deconfigure" ]; then
    if command -v systemctl >/dev/null 2>&1; then
        systemctl stop tunnerse-server.service || true
        systemctl disable tunnerse-server.service || true
    fi
fi

exit 0
EOF

    cat > "$debian_dir/postrm" <<'EOF'
#!/bin/sh
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
fi

if [ "$1" = "purge" ]; then
    rm -rf /etc/tunnerse
fi

exit 0
EOF

    chmod 0755 "$debian_dir/preinst" "$debian_dir/postinst" "$debian_dir/prerm" "$debian_dir/postrm"

    cat > "$pkg_root/usr/share/doc/tunnerse-server/README.Debian" <<'EOF'
Tunnerse public server Debian package
-------------------------------------

This package installs:

  /usr/bin/tunnerse-server
  /lib/systemd/system/tunnerse-server.service
  /etc/tunnerse/tunnerse.config.example

Before starting the service, create:

  /etc/tunnerse/tunnerse.config

Useful commands:

  sudo systemctl enable tunnerse-server
  sudo systemctl start tunnerse-server
  sudo systemctl status tunnerse-server
  sudo journalctl -u tunnerse-server -f
EOF
    gzip -n -9 "$pkg_root/usr/share/doc/tunnerse-server/README.Debian"

    echo "[server:$arch] Building .deb..."
    mkdir -p "$DIST_DIR"
    dpkg-deb --build --root-owner-group "$pkg_root" "$package_output"
}

for arch in $PACKAGE_ARCHES; do
    build_local_package "$arch"
done

for arch in $SERVER_ARCHES; do
    build_server_package "$arch"
done

echo
echo "Build complete."
echo
echo "Debian packages:"
for arch in $PACKAGE_ARCHES; do
    artifact_arch="$(artifact_arch_label "$arch")"
    echo "  $DIST_DIR/tunnerse-installer_${artifact_arch}_${VERSION}.deb"
done
echo
echo "Server Debian packages:"
for arch in $SERVER_ARCHES; do
    artifact_arch="$(artifact_arch_label "$arch")"
    echo "  $DIST_DIR/tunnerse-server-installer_${artifact_arch}_${VERSION}.deb"
done
