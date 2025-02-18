package main

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	progressMap = make(map[string]chan string)
	progressMux sync.Mutex
)

type Episode struct {
	Title    string
	File     string
	Duration string
	PubDate  string
}

type PageData struct {
	Episodes []Episode
	Message  string
	Error    string
}

type ConvertResponse struct {
	SessionId string `json:"sessionId"`
}

func main() {
	mp3Dir, _ := filepath.Abs("mp3s")
	if err := os.MkdirAll(mp3Dir, 0755); err != nil {
		log.Fatalf("Failed to create mp3s directory: %v", err)
	}

	// Set up HTTP routes
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/convert", handleConvert)
	http.HandleFunc("/progress", handleProgress)
	http.HandleFunc("/feed", handleFeed)
	http.HandleFunc("/mp3s/", serveMP3)

	// Start the server
	log.Printf("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.URL.Path == "/delete" {
		filename := r.FormValue("filename")
		if filename != "" {
			mp3Dir, _ := filepath.Abs("mp3s")
			err := os.Remove(filepath.Join(mp3Dir, filename))
			if err != nil {
				http.Redirect(w, r, "/?error=Failed to delete file", http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/?message=File deleted successfully", http.StatusSeeOther)
			return
		}
	}

	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	episodes := getEpisodes()
	data := PageData{
		Episodes: episodes,
		Message:  r.URL.Query().Get("message"),
		Error:    r.URL.Query().Get("error"),
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func handleConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	url := r.FormValue("url")
	if url == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	// Validate YouTube URL
	if !strings.Contains(url, "youtube.com/") && !strings.Contains(url, "youtu.be/") {
		http.Error(w, "Invalid YouTube URL", http.StatusBadRequest)
		return
	}

	// Create a unique session ID
	sessionId := uuid.New().String()
	ch := make(chan string, 10)

	progressMux.Lock()
	progressMap[sessionId] = ch
	progressMux.Unlock()

	// Start conversion in background
	go convertVideo(url, ch, sessionId)

	// Return session ID to client
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ConvertResponse{SessionId: sessionId}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func handleProgress(w http.ResponseWriter, r *http.Request) {
	sessionId := r.URL.Query().Get("id")
	if sessionId == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	progressMux.Lock()
	ch, exists := progressMap[sessionId]
	progressMux.Unlock()

	if !exists {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Stream progress updates
	for msg := range ch {
		fmt.Fprintf(w, "data: %s\n\n", msg)
		flusher.Flush()
	}
}

func convertVideo(url string, ch chan string, sessionId string) {
	defer func() {
		progressMux.Lock()
		delete(progressMap, sessionId)
		progressMux.Unlock()
		close(ch)
	}()

	ch <- "Starting download..."

	// Create temporary directory for download
	tmpDir, err := os.MkdirTemp("", "youtube-dl-*")
	if err != nil {
		ch <- fmt.Sprintf("Error: Failed to create temp directory: %v", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	// Get video title first
	titleCmd := exec.Command("yt-dlp", "--print", "%(title)s", url)
	titleBytes, err := titleCmd.Output()
	if err != nil {
		ch <- fmt.Sprintf("Error: Failed to get video title: %v", err)
		return
	}
	videoTitle := strings.TrimSpace(string(titleBytes))

	// Add file size check before download
	sizeCmd := exec.Command("yt-dlp", "--print", "%(filesize,filesize_approx)s", url)
	sizeBytes, err := sizeCmd.Output()
	if err == nil {
		size, err := strconv.ParseInt(strings.TrimSpace(string(sizeBytes)), 10, 64)
		if err == nil && size > 500*1024*1024 { // 500MB limit
			ch <- "Error: File too large (max 500MB)"
			return
		}
	}

	// Download video using yt-dlp with a sanitized temporary filename
	downloadCmd := exec.Command("yt-dlp",
		"--extract-audio",
		"--audio-format", "mp3",
		"--audio-quality", "0",
		"--restrict-filenames",
		"--progress",
		"--output", filepath.Join(tmpDir, "%(id)s.%(ext)s"),
		"--no-playlist",
		url,
	)

	// Set up output streaming with WaitGroup
	var wg sync.WaitGroup
	stdout, err := downloadCmd.StdoutPipe()
	if err != nil {
		ch <- fmt.Sprintf("Error: Failed to create stdout pipe: %v", err)
		return
	}

	stderr, err := downloadCmd.StderrPipe()
	if err != nil {
		ch <- fmt.Sprintf("Error: Failed to create stderr pipe: %v", err)
		return
	}

	if err := downloadCmd.Start(); err != nil {
		ch <- fmt.Sprintf("Error: Failed to start download: %v", err)
		return
	}

	// Stream output to client
	wg.Add(2)
	go func() {
		defer wg.Done()
		streamOutput(stdout, ch)
	}()
	go func() {
		defer wg.Done()
		streamOutput(stderr, ch)
	}()

	// Wait for command to complete
	if err := downloadCmd.Wait(); err != nil {
		ch <- fmt.Sprintf("Error: Download failed: %v", err)
		return
	}

	// Wait for output streaming to complete
	wg.Wait()

	// Move MP3 file to final location with proper title
	files, err := filepath.Glob(filepath.Join(tmpDir, "*.mp3"))
	if err != nil || len(files) == 0 {
		ch <- "Error: No MP3 file found after conversion"
		return
	}

	sourceFile := files[0]
	mp3Dir, _ := filepath.Abs("mp3s")
	// Sanitize the video title for the filesystem
	safeTitle := strings.Map(func(r rune) rune {
		// Replace problematic characters with safe alternatives
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			return '-'
		default:
			return r
		}
	}, videoTitle)
	destFile := filepath.Join(mp3Dir, safeTitle+".mp3")

	if err := os.Rename(sourceFile, destFile); err != nil {
		ch <- fmt.Sprintf("Error: Failed to move file: %v", err)
		return
	}

	ch <- "Conversion complete!"
	ch <- "DONE"
}

func streamOutput(r io.Reader, ch chan string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		ch <- scanner.Text()
	}
}

func handleFeed(w http.ResponseWriter, r *http.Request) {
	episodes := getEpisodes()
	host := r.Host

	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
    <channel>
        <title>%s</title>
        <link>http://%s</link>
        <description>%s</description>
        <language>en-us</language>
        <lastBuildDate>%s</lastBuildDate>`,
		escapeXML("YouTube to Podcast Converter"),
		escapeXML(host),
		escapeXML("Converted YouTube videos"),
		time.Now().Format(time.RFC1123Z))

	for _, episode := range episodes {
		fmt.Fprintf(w, `
        <item>
            <title>%s</title>
            <description>%s</description>
            <enclosure url="http://%s/mp3s/%s" type="audio/mpeg" />
            <guid>http://%s/mp3s/%s</guid>
            <pubDate>%s</pubDate>
            <duration>%s</duration>
        </item>`,
			escapeXML(episode.Title),
			escapeXML("Audio file converted from YouTube"),
			escapeXML(host),
			escapeXML(episode.File),
			escapeXML(host),
			escapeXML(episode.File),
			episode.PubDate,
			episode.Duration)
	}

	fmt.Fprintf(w, `
    </channel>
</rss>`)
}

func getEpisodes() []Episode {
	mp3Dir, _ := filepath.Abs("mp3s")
	files, err := filepath.Glob(filepath.Join(mp3Dir, "*.mp3"))
	if err != nil {
		return nil
	}

	var episodes []Episode
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		episodes = append(episodes, Episode{
			Title:    strings.TrimSuffix(filepath.Base(file), ".mp3"),
			File:     filepath.Base(file),
			Duration: getDuration(file),
			PubDate:  info.ModTime().Format(time.RFC1123Z),
		})
	}

	return episodes
}

func getDuration(file string) string {
	if !filepath.IsAbs(file) {
		mp3Dir, _ := filepath.Abs("mp3s")
		file = filepath.Join(mp3Dir, filepath.Base(file))
	}

	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		file)

	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	seconds := strings.TrimSpace(string(output))
	duration, err := time.ParseDuration(seconds + "s")
	if err != nil {
		return "unknown"
	}

	minutes := int(duration.Minutes())
	remainingSeconds := int(duration.Seconds()) % 60

	return fmt.Sprintf("%d:%02d", minutes, remainingSeconds)
}

func serveMP3(w http.ResponseWriter, r *http.Request) {
	filename := filepath.Base(r.URL.Path)
	mp3Dir, _ := filepath.Abs("mp3s")
	http.ServeFile(w, r, filepath.Join(mp3Dir, filename))
}

func escapeXML(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return s // Return original string if escaping fails
	}
	return b.String()
}

// Add new function for cleaning old files
