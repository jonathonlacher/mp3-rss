package main

import (
	"bufio"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Episode struct {
	Title    string
	File     string
	PubDate  string
	Duration string
	Size     int64
}

type PageData struct {
	Message  string
	Error    string
	Episodes []Episode
}

func main() {
	// Create mp3s directory if it doesn't exist
	if err := os.MkdirAll("mp3s", 0755); err != nil {
		log.Fatal("Failed to create mp3s directory:", err)
	}

	// Setup progress channel for conversion updates
	progressChan := make(chan string, 10)
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

	log.Println("Server starting at http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	episodes, err := getEpisodes()
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := PageData{
		Message:  r.URL.Query().Get("message"),
		Error:    r.URL.Query().Get("error"),
		Episodes: episodes,
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func handleProgress(progressChan <-chan string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
			return
		}

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
		http.Redirect(w, r, "/?error=No URL provided", http.StatusSeeOther)
		return
	}

	go func() {
		if err := convertVideo(youtubeURL, progressChan); err != nil {
			progressChan <- "Error: " + err.Error()
			return
		}
		progressChan <- "Conversion complete!"
		progressChan <- "DONE"
	}()

	http.Redirect(w, r, "/?message=Conversion started", http.StatusSeeOther)
}

func convertVideo(youtubeURL string, progressChan chan<- string) error {
	progressChan <- "Starting download..."

	cmd := exec.Command("yt-dlp",
		"--extract-audio",
		"--audio-format", "mp3",
		"--audio-quality", "320",
		"--embed-metadata",
		"--output", "mp3s/%(title)s.%(ext)s",
		youtubeURL)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start download: %v", err)
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.Contains(line, "[download]"):
				progressChan <- "Downloading..."
			case strings.Contains(line, "[ExtractAudio]"):
				progressChan <- "Converting to MP3..."
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("conversion failed: %v", err)
	}

	return nil
}

func handleRSS(w http.ResponseWriter, r *http.Request) {
	episodes, err := getEpisodes()
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	host := r.Host
	rss := generateRSSFeed(host, episodes)

	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprint(w, rss)
}

func generateRSSFeed(host string, episodes []Episode) string {
	rss := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
    <rss version="2.0"
         xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
        <channel>
            <title>YouTube Downloads</title>
            <link>http://%s</link>
            <description>YouTube videos converted to MP3</description>
            <language>en-us</language>`, host)

	for _, episode := range episodes {
		rss += fmt.Sprintf(`
            <item>
                <title>%s</title>
                <enclosure url="http://%s/mp3s/%s" length="%d" type="audio/mpeg"/>
                <guid>http://%s/mp3s/%s</guid>
                <pubDate>%s</pubDate>
                <itunes:duration>%s</itunes:duration>
            </item>`,
			episode.Title,
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
	files, err := os.ReadDir("mp3s")
	if err != nil {
		return nil, fmt.Errorf("failed to read mp3s directory: %v", err)
	}

	var episodes []Episode
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".mp3") {
			continue
		}

		info, err := f.Info()
		if err != nil {
			continue
		}

		duration, _ := getAudioDuration(filepath.Join("mp3s", f.Name()))
		if duration == "" {
			duration = "Unknown"
		}

		episodes = append(episodes, Episode{
			Title:    strings.TrimSuffix(f.Name(), ".mp3"),
			File:     f.Name(),
			PubDate:  info.ModTime().Format(time.RFC1123Z),
			Duration: duration,
			Size:     info.Size(),
		})
	}

	return episodes, nil
}

func getAudioDuration(filepath string) (string, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filepath)

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	var seconds float64
	fmt.Sscanf(string(output), "%f", &seconds)

	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs), nil
}
