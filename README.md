# mp3-rss

A simple web application that converts YouTube videos to MP3s and serves them via an RSS feed for podcast apps.

## Features

- Converts YouTube videos to high-quality MP3s
- Optional audio normalization to make volume levels consistent
- Serves MP3s via RSS feed compatible with podcast apps
- Simple web interface for managing conversions and episodes

## Installation

1. First-time VM setup:

    ```bash
    # Copy the setup script to VM
    scp scripts/vm-setup.sh root@<VM_IP>:setup.sh

    # SSH to VM and run setup
    ssh root@<VM_IP> "bash setup.sh"
    ```

2. Edit `.env`:

    ```env
    VM_IP="your-vm-ip"
    ```

## Deployment

From local:

```bash
./scripts/deploy.sh
```

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
- Periodically update `yt-dlp` using the update script:

    ```bash
    ./scripts/update-assets.sh
    ```

## Troubleshooting

- Check `systemd` service status:

  ```bash
  ssh root@<VM_IP> "systemctl status youtube-podcast"
  ```

- View logs:

  ```bash
  ssh root@<VM_IP> "journalctl -u youtube-podcast -f"
  ```
