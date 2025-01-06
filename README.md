# mp3-rss

A simple web application that converts YouTube videos to MP3s and serves them via an RSS feed for podcast apps.

## Features

- Convert YouTube videos to MP3
- Web interface for easy conversion
- RSS feed for podcast apps
- Progress tracking during conversion

## Prerequisites

On the VM:
- Ubuntu/Debian
- Tailscale installed and configured
- SSH access configured

## Installation

1. First-time VM setup:

    ```bash
    # Copy the setup script to VM
    scp scripts/setup.sh root@<VM_IP>:

    # SSH to VM and run setup
    ssh root@<VM_IP> "bash setup.sh"
    ```

    This installs required dependencies:
    - ffmpeg
    - python3-pip
    - yt-dlp
    - Creates necessary directories

2. Edit your `.env`:

    ```env
    VM_IP="vm-ips"
    ```

## Deployment

From your Mac, just run:

```bash
./scripts/deploy.sh
```

This will:

1. Build the Linux binary
2. Copy files to the VM
3. Set up/update the systemd service
4. Restart the application

## Usage

1. Access the web interface:

    ```text
    http://<IP>:8080
    ```

2. Enter a YouTube URL and click Convert
3. Wait for conversion to complete
4. Copy the RSS feed URL for your podcast app

## Maintenance

- Keep an eye on disk usage in `/opt/youtube-podcast/mp3s`
- Periodically update yt-dlp:
  
    ```bash
    ssh root@VM_IP "pip3 install -U yt-dlp"
    ```

## Troubleshooting

- Check systemd service status:
  
  ```bash
  ssh root@VM_IP "systemctl status youtube-podcast"
  ```

- View logs:
  
  ```bash
  ssh root@VM_IP "journalctl -u youtube-podcast -f"
  ```
