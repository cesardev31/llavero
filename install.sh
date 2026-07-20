#!/bin/bash

set -e

REPO="orvixapp/llavero"
RAW_BASE_URL="https://raw.githubusercontent.com/$REPO/main"

# Parsear argumentos
INSTALL_SERVER=true
INSTALL_CLI=true

for arg in "$@"; do
  case $arg in
    --cli-only)
      INSTALL_SERVER=false
      INSTALL_CLI=true
      shift
      ;;
    --server-only)
      INSTALL_SERVER=true
      INSTALL_CLI=false
      shift
      ;;
    --help)
      echo "Usage: $0 [--cli-only | --server-only]"
      exit 0
      ;;
  esac
done

echo "🔑 Llavero - Installation Script"
echo "=================================="

# Verificar si se corre como root
if [ "$EUID" -ne 0 ]; then
  echo "Please run this script as root (e.g. sudo ./install.sh)"
  exit 1
fi

# Detectar arquitectura
ARCH=$(uname -m)
case $ARCH in
    x86_64) GOARCH="amd64" ;;
    aarch64|arm64) GOARCH="arm64" ;;
    *) echo "Architecture $ARCH is not supported."; exit 1 ;;
esac

echo "1. Fetching latest release from GitHub..."
TAG=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$TAG" ]; then
    echo "No releases found in the $REPO repository."
    echo "Make sure you have created at least one release (tag vX.X.X)."
    exit 1
fi

echo "Latest release found: $TAG"

echo "2. Downloading checksums..."
curl -sL "https://github.com/$REPO/releases/download/$TAG/checksums.txt" -o /tmp/llavero-checksums.txt

if [ "$INSTALL_SERVER" = true ]; then
    echo "3. Downloading Server binary (llavero) for linux-$GOARCH..."
    curl -sL "https://github.com/$REPO/releases/download/$TAG/llavero-linux-$GOARCH" -o /tmp/llavero-linux-$GOARCH
    echo "   Verifying checksum..."
    (cd /tmp && grep "llavero-linux-$GOARCH" llavero-checksums.txt | sha256sum -c --quiet)
    mv /tmp/llavero-linux-$GOARCH /usr/local/bin/llavero
    chmod +x /usr/local/bin/llavero
fi

if [ "$INSTALL_CLI" = true ]; then
    echo "4. Downloading CLI binary (llavero-cli) for linux-$GOARCH..."
    curl -sL "https://github.com/$REPO/releases/download/$TAG/llavero-cli-linux-$GOARCH" -o /tmp/llavero-cli-linux-$GOARCH
    echo "   Verifying checksum..."
    (cd /tmp && grep "llavero-cli-linux-$GOARCH" llavero-checksums.txt | sha256sum -c --quiet)
    mv /tmp/llavero-cli-linux-$GOARCH /usr/local/bin/llavero-cli
    chmod +x /usr/local/bin/llavero-cli
fi

rm -f /tmp/llavero-checksums.txt

if [ "$INSTALL_SERVER" = true ]; then
    echo "3. Creating system user and group (llavero)..."
    if ! id "llavero" &>/dev/null; then
        useradd --system --no-create-home --user-group llavero
    fi

    echo "4. Creating directories and configurations..."
    # Directorio de datos
    mkdir -p /var/lib/llavero
    chown -R llavero:llavero /var/lib/llavero
    chmod 750 /var/lib/llavero

    # Directorio de configuración
    mkdir -p /etc/llavero
    if [ ! -f /etc/llavero/llavero.conf ]; then
        echo "Downloading base configuration file..."
        curl -sL "$RAW_BASE_URL/deploy/llavero.conf.example" -o /etc/llavero/llavero.conf
    fi

    if [ ! -f /etc/llavero/llavero.env ]; then
        touch /etc/llavero/llavero.env
    fi

    chown -R llavero:llavero /etc/llavero
    chmod 750 /etc/llavero
    chmod 640 /etc/llavero/llavero.conf /etc/llavero/llavero.env

    echo "5. Downloading, installing and enabling systemd service..."
    curl -sL "$RAW_BASE_URL/deploy/llavero.service" -o /etc/systemd/system/llavero.service
    systemctl daemon-reload
    systemctl enable llavero.service
    systemctl restart llavero.service
fi

echo ""
echo "✅ Installation completed successfully."
echo "--------------------------------------------------------"
if [ "$INSTALL_SERVER" = true ]; then
    echo "‣ Llavero Server ($TAG) is now running in the background as a service."
    echo "‣ You can view the logs using: journalctl -u llavero -f"
    echo "‣ The configuration file is located at: /etc/llavero/llavero.conf"
fi
if [ "$INSTALL_CLI" = true ]; then
    echo "‣ You can interact with the console by running: llavero-cli"
fi
echo "--------------------------------------------------------"
