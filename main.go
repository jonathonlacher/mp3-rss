package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
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

	// Create mp3s directory
	if err := os.MkdirAll("mp3s", 0755); err != nil {
		log.Fatalf("Failed to create mp3s directory: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/convert", handleConvert)
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

func handleConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	youtubeURL := r.FormValue("url")
	if youtubeURL == "" {
		http.Redirect(w, r, "/?error="+ErrInvalidURL.Error(), http.StatusSeeOther)
		return
	}

	if err := convertVideo(youtubeURL); err != nil {
		log.Printf("Conversion error: %v", err)
		http.Redirect(w, r, "/?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/?message=Conversion successful", http.StatusSeeOther)
}

func convertVideo(youtubeURL string) error {
	cmd := exec.Command("yt-dlp",
		"--extract-audio",
		"--audio-format", "mp3",
		"--audio-quality", "320",
		"--embed-metadata",
		"--add-metadata",
		"--write-info-json",
		"--write-description",
		"--embed-chapters",
		"--output", "mp3s/%(title)s.%(ext)s",
		youtubeURL)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %v - %s", ErrConversion, err, string(output))
	}

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
		jsonPath := filepath.Join("mp3s", baseName+".info.json")
		descPath := filepath.Join("mp3s", baseName+".description")

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
