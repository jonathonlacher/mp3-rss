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

# Build for Linux with embedded static files
echo "Building for Linux with embedded static files..."
GOOS=linux GOARCH=amd64 go build -o youtube-podcast

# First copy to /tmp
echo "Copying binary to /tmp first..."
scp youtube-podcast root@"$VM_IP":/tmp/

# Then move it to final location
echo "Moving binary to final location..."
ssh root@"$VM_IP" "mv /tmp/youtube-podcast $APP_DIR/ && chmod 755 $APP_DIR/youtube-podcast"


# Setup systemd service
echo "Setting up systemd service..."
ssh root@"$VM_IP" "cat > /etc/systemd/system/youtube-podcast.service << 'EOL'
[Unit]
Description=YouTube to Podcast Converter
After=network.target

[Service]
Type=simple
User=root
Environment=PATH=/root/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/youtube-podcast
Restart=always

[Install]
WantedBy=multi-user.target
EOL
systemctl daemon-reload && systemctl enable youtube-podcast"

# Restart service
echo "Restarting service..."
ssh root@"$VM_IP" "systemctl restart youtube-podcast"

echo "Checking service status..."
ssh root@"$VM_IP" "systemctl status youtube-podcast"

echo "Deployment complete!"
