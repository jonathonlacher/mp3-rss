# Notes

This file is for myself and the LLMs.

## Project purpose

The purpose of this project is to make it easy to save videos, such as DJ sets, interviews, or talks, and make them available as a private audio podcast feed for myself.

I host the project locally on my network, which means the feed is available only to me when my devices are connected to my network.

## Features

- Converts given YouTube video URLs to MP3s and saves these to a local directory
- Serves the MP3s via an RSS feed
- From the web interface, users can:
  - Enter a YouTube URL
  - Click a button to convert the video
  - Copy the RSS feed URL for their podcast app
  - View the list of converted videos
  - Play back the MP3s in the browser
  - Delete MP3s
  - Apply audio normalization to make volume levels consistent

## TODO

- [x] Add option to normalize audio levels (Completed: April 2025)
- [ ] Extract and display chapters from YouTube videos if available

## Development Guidelines

### Build Commands

- `make build` - Build binary locally
- `make run` - Run application
- `make test` - Run all tests
- `make test-race` - Run tests with race detection
- `make test-coverage` - Run tests with coverage
- `make lint` - Run golangci-lint
- `make clean` - Clean up artifacts
- `make build-linux` - Cross-compile for Linux
- `./scripts/deploy.sh` - Build and deploy to VM

### Code Style

- Imports: Standard library first, then third-party packages, alphabetically ordered
- Naming: camelCase for variables, MixedCase for functions, methods prefixed with receiver
- Error handling: Explicit checks, use fmt.Errorf for context, structured messages
- Comments: Start with entity name, document exported functions, use full sentences
- Types: Define structs with purpose comments, organize fields logically
- Functions: Follow single responsibility principle, favor dependency injection
- Formatting: Use standard Go formatting (gofmt), 4-space indentation
- Testing: Write unit tests for all new functionality, mock dependencies when needed

### Requirements

- Go 1.22 or higher
- FFmpeg with ffprobe
- yt-dlp

### Implementation Notes

- Audio normalization uses FFmpeg's loudnorm filter with I=-16:LRA=11:TP=-1.5
- Files are processed in temporary directories to avoid partial downloads
- File names are sanitized and timestamps added to avoid conflicts
