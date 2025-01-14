package main

import (
	"bufio"
	"encoding/json"
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
	Title      string
	File       string
	PubDate    string
	Duration   string
	Size       int64
	BitRate    string
	Channels   string
	SampleRate string
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
	mux.Handle("/mp3s/", http.StripPrefix("/mp3s/", serveMp3Files()))

	log.Println("Server starting at http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}

func serveMp3Files() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "audio/mpeg")
		http.FileServer(http.Dir("mp3s")).ServeHTTP(w, r)
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
					// Channel closed; exit the handler
					return
				}
				// Send progress updates to the client
				fmt.Fprintf(w, "data: %s\n\n", msg)
				flusher.Flush()
			case <-notify:
				// Client disconnected; exit the handler
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

	// Start the conversion in a goroutine
	go func() {
		if err := convertVideo(youtubeURL, progressChan); err != nil {
			progressChan <- fmt.Sprintf("Error: %v", err)
			log.Println(err) // Log the error for debugging
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
		"--embed-thumbnail",
		"--output", "mp3s/%(title)s.%(ext)s",
		youtubeURL)

	// Create pipes for stdout and stderr
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start download: %v", err)
	}

	// Capture stdout and stderr concurrently
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			progressChan <- line // Send stdout lines to the UI
		}
		if err := scanner.Err(); err != nil {
			log.Printf("Error reading stdout: %v", err)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			progressChan <- line // Send stderr lines to the UI
		}
		if err := scanner.Err(); err != nil {
			log.Printf("Error reading stderr: %v", err)
		}
	}()

	// Wait for the command to complete
	if err := cmd.Wait(); err != nil {
		progressChan <- fmt.Sprintf("Error: %v", err)
		return fmt.Errorf("conversion failed: %v", err)
	}

	progressChan <- "Download and conversion successful!"
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
         xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"
         xmlns:atom="http://www.w3.org/2005/Atom"
         xmlns:content="http://purl.org/rss/1.0/modules/content/"
         xmlns:media="http://search.yahoo.com/mrss/">
        <channel>
            <title>YouTube Downloads</title>
            <link>http://%[1]s</link>
            <atom:link href="http://%[1]s/feed" rel="self" type="application/rss+xml" />
            <description>YouTube videos converted to MP3</description>
            <language>en-us</language>
            <itunes:author>YouTube Downloads</itunes:author>
            <itunes:type>episodic</itunes:type>
            <itunes:category text="Technology"/>`, host)

	for _, episode := range episodes {
		rss += fmt.Sprintf(`
            <item>
                <title>%s</title>
                <enclosure url="http://%s/mp3s/%s" 
                          length="%d" 
                          type="audio/mpeg"/>
                <link>http://%s/mp3s/%s</link>
                <guid isPermaLink="true">http://%s/mp3s/%s</guid>
                <pubDate>%s</pubDate>
                <itunes:duration>%s</itunes:duration>
                <itunes:explicit>no</itunes:explicit>
                <itunes:episodeType>full</itunes:episodeType>
                <description>Audio file converted from YouTube</description>
                <media:content url="http://%s/mp3s/%s" 
                             fileSize="%d" 
                             type="audio/mpeg" 
                             duration="%s"/>
                <content:encoded>
                    <![CDATA[
                    Audio Details:<br/>
                    Bit Rate: %s<br/>
                    Channels: %s<br/>
                    Sample Rate: %s
                    ]]>
                </content:encoded>
            </item>`,
			episode.Title,
			host, episode.File, episode.Size,
			host, episode.File,
			host, episode.File,
			episode.PubDate,
			episode.Duration,
			host, episode.File, episode.Size, episode.Duration,
			episode.BitRate,
			episode.Channels,
			episode.SampleRate)
	}

	rss += `
        </channel>
    </rss>`

	return rss
}

func getAudioMetadata(filepath string) (map[string]string, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filepath)

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var data struct {
		Streams []struct {
			CodecType  string `json:"codec_type"`
			BitRate    string `json:"bit_rate"`
			Channels   int    `json:"channels"`
			SampleRate string `json:"sample_rate"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			BitRate  string `json:"bit_rate"`
		} `json:"format"`
	}

	if err := json.Unmarshal(output, &data); err != nil {
		return nil, err
	}

	metadata := make(map[string]string)
	metadata["duration"] = data.Format.Duration
	metadata["bit_rate"] = data.Format.BitRate

	// Look for audio stream
	for _, stream := range data.Streams {
		if stream.CodecType == "audio" {
			if stream.BitRate != "" {
				metadata["bit_rate"] = stream.BitRate
			}
			metadata["channels"] = fmt.Sprintf("%d", stream.Channels)
			metadata["sample_rate"] = stream.SampleRate
			break
		}
	}

	return metadata, nil
}

func getAudioDuration(filepath string) (string, error) {
	metadata, err := getAudioMetadata(filepath)
	if err != nil {
		return "", err
	}

	duration, ok := metadata["duration"]
	if !ok {
		return "", fmt.Errorf("no duration found in metadata")
	}

	var seconds float64
	_, err = fmt.Sscanf(duration, "%f", &seconds)
	if err != nil {
		return "", fmt.Errorf("failed to parse duration: %v", err)
	}

	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs), nil
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

		filepath := filepath.Join("mp3s", f.Name())
		metadata, err := getAudioMetadata(filepath)
		if err != nil {
			log.Printf("Warning: Could not get metadata for %s: %v", f.Name(), err)
		}

		duration, _ := getAudioDuration(filepath)
		if duration == "" {
			duration = "Unknown"
		}

		episode := Episode{
			Title:    strings.TrimSuffix(f.Name(), ".mp3"),
			File:     f.Name(),
			PubDate:  info.ModTime().Format(time.RFC1123Z),
			Duration: duration,
			Size:     info.Size(),
		}

		if metadata != nil {
			episode.BitRate = metadata["bit_rate"]
			episode.Channels = metadata["channels"]
			episode.SampleRate = metadata["sample_rate"]
		}

		episodes = append(episodes, episode)
	}

	return episodes, nil
}
