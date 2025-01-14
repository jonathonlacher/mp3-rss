#!/bin/bash

# Exit on any error
set -e

# Ensure the script runs relative to the repo root
cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 1

# Load environment variables if the .env file exists
if [[ -f .env ]]; then
    # shellcheck source=.env
    source .env
fi

# Default values for environment variables (can be overridden by user-provided env vars)
VM_IP="${VM_IP:-}"
APP_DIR="${APP_DIR:-/opt/youtube-podcast}"

# Print configuration for debugging
echo "Using VM_IP=${VM_IP}"
echo "Using APP_DIR=${APP_DIR}"

# Update the assets on the remote server
echo "Updating assets on the remote server..."
ssh root@"$VM_IP" "apt update && apt upgrade -y"

# Update yt-dlp using pipx
echo "Updating yt-dlp using pipx..."
ssh root@"$VM_IP" "pipx install yt-dlp --force"

# Restart the service if needed
read -p "Do you want to restart the service? [y/N]: " -r
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Restarting service..."
    ssh root@"$VM_IP" "systemctl restart youtube-podcast"
else
    echo "Skipping service restart."
fi

echo "Update complete!"
