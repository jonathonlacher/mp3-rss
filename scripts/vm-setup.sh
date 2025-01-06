#!/bin/bash

# Update and install dependencies
apt update && apt upgrade -y
apt install -y ffmpeg python3-pip python3-venv pipx

# Ensure pipx is in PATH and complete installation
pipx ensurepath
export PATH="/root/.local/bin:$PATH"

# Install yt-dlp using pipx
pipx install yt-dlp

# Create app directory structure
mkdir -p /opt/youtube-podcast/{templates,mp3s}
chmod 755 /opt/youtube-podcast

# Verify yt-dlp installation
which yt-dlp
