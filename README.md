# mp3-rss

A simple web application that converts YouTube videos to MP3s and serves them via an RSS feed for podcast apps.

## Features

- Convert YouTube videos to MP3
- Web interface for easy conversion
- RSS feed for podcast apps
- Progress tracking during conversion
- Accessible via Tailscale private network

## Prerequisites

On your development machine (Mac):
- Go 1.21+
- Git

On the VM:
- Ubuntu/Debian
- Tailscale installed and configured
- SSH access configured

## Installation

1. First-time VM setup:

    ```bash
    # Copy the setup script to VM
    scp setup.sh root@VM_IP:

    # SSH to VM and run setup
    ssh root@VM_IP "bash setup.sh"
    ```

This installs required dependencies:
- ffmpeg
- python3-pip
- yt-dlp
- Creates necessary directories

2. Edit the deployment script:

    ```bash
    # Edit deploy.sh and set your VM's Tailscale IP
    VM_IP="your-vm-tailscale-ip"
    ```

## Development

The project structure:
```
.
├── main.go
├── templates/
│   └── index.html
├── setup.sh
└── deploy.sh
```

## Deployment

From your Mac, just run:

```bash
./deploy.sh
```

This will:

1. Build the Linux binary
2. Copy files to the VM
3. Set up/update the systemd service
4. Restart the application

## Usage

1. Access the web interface:

    ```text
    http://VM_TAILSCALE_IP:8080
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

## Directory Structure on VM

```text
/opt/youtube-podcast/
├── youtube-podcast (binary)
├── templates/
│   └── index.html
└── mp3s/
    └── (downloaded files)
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

## Notes

- The application is only accessible via Tailscale
- MP3s are stored locally on the VM
- No authentication is implemented - secure via Tailscale network only
