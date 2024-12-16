package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type VideoMetadata struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Uploader    string    `json:"uploader"`
	Duration    float64   `json:"duration"`
	Chapters    []Chapter `json:"chapters"`
}

type Chapter struct {
	StartTime float64 `json:"start_time"`
	Title     string  `json:"title"`
}

type Episode struct {
	Title       string
	File        string
	Description string
	Uploader    string
	PubDate     string
	Duration    string
	Size        int64
	Chapters    []Chapter
}

type PageData struct {
	Message  string
	Error    string
	Episodes []Episode
}

var (
	ErrInvalidURL    = errors.New("invalid or empty URL provided")
	ErrConversion    = errors.New("failed to convert video")
	ErrMissingYTDLP  = errors.New("yt-dlp is not installed")
	ErrMissingFFmpeg = errors.New("ffmpeg is not installed")
)

func main() {
	if err := checkDependencies(); err != nil {
		log.Fatal(err)
	}

	// Create required directories
	for _, dir := range []string{"mp3s", "metadata"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Setup progress channel for conversion updates
	progressChan := make(chan string, 100)
	defer close(progressChan)

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/convert", func(w http.ResponseWriter, r *http.Request) {
		handleConvert(w, r, progressChan)
	})
	mux.HandleFunc("/progress", handleProgress(progressChan))
	mux.HandleFunc("/feed", handleRSS)
	mux.Handle("/mp3s/", http.StripPrefix("/mp3s/", http.FileServer(http.Dir("mp3s"))))

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Println("Server starting at http://localhost:8080")
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func checkDependencies() error {
	// Check for yt-dlp
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return fmt.Errorf("%w: %v", ErrMissingYTDLP, err)
	}

	// Check for ffmpeg
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("%w: %v", ErrMissingFFmpeg, err)
	}

	return nil
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	funcMap := template.FuncMap{
		"formatDuration": func(seconds float64) string {
			hours := int(seconds) / 3600
			minutes := (int(seconds) % 3600) / 60
			secs := int(seconds) % 60
			return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
		},
	}

	tmpl, err := template.New("index.html").Funcs(funcMap).ParseFiles("templates/index.html")
	if err != nil {
		log.Printf("Template parsing error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	episodes, err := getEpisodes()
	if err != nil {
		log.Printf("Error getting episodes: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := PageData{
		Message:  r.URL.Query().Get("message"),
		Error:    r.URL.Query().Get("error"),
		Episodes: episodes,
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func handleProgress(progressChan <-chan string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set headers for SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
			return
		}

		// Create a channel for disconnect detection
		notify := r.Context().Done()

		for {
			select {
			case msg, ok := <-progressChan:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", msg)
				flusher.Flush()
			case <-notify:
				// Client disconnected
				return
			}
		}
	}
}

func handleConvert(w http.ResponseWriter, r *http.Request, progressChan chan<- string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	youtubeURL := r.FormValue("url")
	if youtubeURL == "" {
		http.Redirect(w, r, "/?error="+ErrInvalidURL.Error(), http.StatusSeeOther)
		return
	}

	// Create a new channel specifically for this conversion
	conversionProgress := make(chan string, 100)

	// Start conversion in a goroutine
	go func() {
		if err := convertVideo(youtubeURL, conversionProgress); err != nil {
			// Send error through progress channel
			conversionProgress <- "Error: " + err.Error()
		}
		close(conversionProgress)
	}()

	// Forward messages from conversion to the main progress channel
	go func() {
		for msg := range conversionProgress {
			progressChan <- msg
			if msg == "Conversion complete!" {
				// Add a small delay before redirecting
				time.Sleep(500 * time.Millisecond)
				progressChan <- "DONE" // Special message to trigger redirect
			}
		}
	}()

	http.Redirect(w, r, "/?message=Conversion started", http.StatusSeeOther)
}

// Update the convertVideo function to improve progress reporting
func convertVideo(youtubeURL string, progressChan chan<- string) error {
	// First, download metadata to get the title
	progressChan <- "Fetching video metadata..."
	tmpDir, err := os.MkdirTemp("", "yt-dl-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	metadataCmd := exec.Command("yt-dlp",
		"--dump-json",
		"--no-download",
		youtubeURL)

	metadataOutput, err := metadataCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to fetch metadata: %w", err)
	}

	var metadata struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(metadataOutput, &metadata); err != nil {
		return fmt.Errorf("failed to parse metadata: %w", err)
	}

	// Sanitize filename
	safeTitle := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '-'
		}
		return r
	}, metadata.Title)

	progressChan <- "Starting download..."

	cmd := exec.Command("yt-dlp",
		"--extract-audio",
		"--audio-format", "mp3",
		"--audio-quality", "320",
		"--embed-metadata",
		"--add-metadata",
		"--embed-chapters",
		"--write-info-json",
		"--write-description",
		"--output", filepath.Join(tmpDir, "%(title)s.%(ext)s"),
		"--progress",
		youtubeURL)

	// Create pipes for stdout and stderr
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: %v", ErrConversion, err)
	}

	// Monitor progress output
	go func() {
		scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
		for scanner.Scan() {
			line := scanner.Text()
			log.Println(line) // Keep logging all lines

			// Send progress updates for download and conversion
			switch {
			case strings.Contains(line, "[download]"):
				if strings.Contains(line, "%") {
					progressChan <- fmt.Sprintf("Downloading: %s", line)
				}
			case strings.Contains(line, "[ExtractAudio]"):
				progressChan <- "Converting to MP3..."
			case strings.Contains(line, "[Metadata]"):
				progressChan <- "Adding metadata..."
			case strings.Contains(line, "Adding metadata to"):
				// This is the last line we see before completion
				progressChan <- "Finalizing..."
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%w: %v", ErrConversion, err)
	}

	// After yt-dlp finishes, proceed with file processing
	progressChan <- "Moving files to final location..."

	// Move files to their respective directories
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		return fmt.Errorf("failed to read temp directory: %w", err)
	}

	for _, file := range files {
		src := filepath.Join(tmpDir, file.Name())
		var dst string

		switch {
		case strings.HasSuffix(file.Name(), ".mp3"):
			dst = filepath.Join("mp3s", safeTitle+".mp3")
			progressChan <- "Saving MP3 file..."
		case strings.HasSuffix(file.Name(), ".info.json"):
			dst = filepath.Join("metadata", safeTitle+".info.json")
		case strings.HasSuffix(file.Name(), ".description"):
			dst = filepath.Join("metadata", safeTitle+".description")
		default:
			continue
		}

		if err := os.Rename(src, dst); err != nil {
			log.Printf("Failed to move file %s: %v", file.Name(), err)
		}
	}

	// Log completion for debugging
	log.Println("Conversion process completed successfully")

	// Send final completion message
	progressChan <- "Conversion complete!"
	return nil
}

func handleRSS(w http.ResponseWriter, r *http.Request) {
	episodes, err := getEpisodes()
	if err != nil {
		log.Printf("Error getting episodes for RSS: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	host := r.Host
	rss := generateRSSFeed(host, episodes)

	w.Header().Set("Content-Type", "application/xml")
	if _, err := fmt.Fprint(w, rss); err != nil {
		log.Printf("Error writing RSS response: %v", err)
	}
}

func generateRSSFeed(host string, episodes []Episode) string {
	rss := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
    <rss version="2.0"
         xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"
         xmlns:content="http://purl.org/rss/1.0/modules/content/">
        <channel>
            <title>YouTube Downloads</title>
            <link>http://%s</link>
            <description>YouTube videos converted to MP3</description>
            <language>en-us</language>`, host)

	for _, episode := range episodes {
		// Create chapter markers in description if available
		chapterText := ""
		if len(episode.Chapters) > 0 {
			chapterText = "\n\nChapters:\n"
			for _, chapter := range episode.Chapters {
				chapterText += fmt.Sprintf("%s - %s\n",
					formatDuration(chapter.StartTime),
					chapter.Title)
			}
		}

		// Combine description and chapters
		fullDescription := episode.Description + chapterText

		rss += fmt.Sprintf(`
            <item>
                <title>%s</title>
                <itunes:author>%s</itunes:author>
                <description><![CDATA[%s]]></description>
                <content:encoded><![CDATA[%s]]></content:encoded>
                <enclosure url="http://%s/mp3s/%s" length="%d" type="audio/mpeg"/>
                <guid>http://%s/mp3s/%s</guid>
                <pubDate>%s</pubDate>
                <itunes:duration>%s</itunes:duration>
            </item>`,
			episode.Title,
			episode.Uploader,
			fullDescription,
			fullDescription,
			host, episode.File, episode.Size,
			host, episode.File,
			episode.PubDate,
			episode.Duration)
	}

	rss += `
        </channel>
    </rss>`

	return rss
}

func getEpisodes() ([]Episode, error) {
	files, err := ioutil.ReadDir("mp3s")
	if err != nil {
		return nil, fmt.Errorf("failed to read mp3s directory: %w", err)
	}

	var episodes []Episode
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".mp3") {
			continue
		}

		baseName := strings.TrimSuffix(f.Name(), ".mp3")
		jsonPath := filepath.Join("metadata", baseName+".info.json")
		descPath := filepath.Join("metadata", baseName+".description")

		episode := Episode{
			Title:   baseName,
			File:    f.Name(),
			PubDate: f.ModTime().Format(time.RFC1123Z),
			Size:    f.Size(),
		}

		// Try to load metadata from JSON
		if metadata, err := loadMetadata(jsonPath); err == nil {
			episode.Duration = formatDuration(metadata.Duration)
			episode.Uploader = metadata.Uploader
			episode.Chapters = metadata.Chapters
			episode.Description = metadata.Description
		} else {
			// Fallback to ffprobe for duration
			if duration, err := getAudioDuration(filepath.Join("mp3s", f.Name())); err == nil {
				episode.Duration = duration
			} else {
				episode.Duration = "Unknown"
			}
		}

		// Try to load description from separate file if not in metadata
		if episode.Description == "" {
			if desc, err := ioutil.ReadFile(descPath); err == nil {
				episode.Description = string(desc)
			}
		}

		episodes = append(episodes, episode)
	}

	return episodes, nil
}

func formatDuration(seconds float64) string {
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
}

func loadMetadata(path string) (*VideoMetadata, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var metadata VideoMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

func getAudioDuration(filepath string) (string, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filepath)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get audio duration: %w", err)
	}

	// Parse duration from output
	var seconds float64
	fmt.Sscanf(string(output), "%f", &seconds)

	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs), nil
}
