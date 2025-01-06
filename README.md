# mp3-rss

A simple web application that converts YouTube videos to MP3s and serves them via an RSS feed for podcast apps.

## Installation

1. First-time VM setup:

    ```bash
    # Copy the setup script to VM
    scp scripts/setup.sh root@<VM_IP>:

    # SSH to VM and run setup
    ssh root@<VM_IP> "bash setup.sh"
    ```

2. Edit `.env`:

    ```env
    VM_IP="vm-ips"
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
- Periodically update `yt-dlp`:
  
    ```bash
    ssh root@<VM_IP> "pip3 install -U yt-dlp"
    ```

## Troubleshooting

- Check systemd service status:
  
  ```bash
  ssh root@<VM_IP> "systemctl status youtube-podcast"
  ```

- View logs:
  
  ```bash
  ssh root@<VM_IP> "journalctl -u youtube-podcast -f"
  ```
