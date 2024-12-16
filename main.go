// main.go
package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Episode struct {
	Title    string
	File     string
	URL      string
	PubDate  string
	Duration string
	Size     int64
}

type PageData struct {
	Message  string
	Episodes []Episode
}

func main() {
	// Create mp3 directory if it doesn't exist
	os.MkdirAll("mp3s", 0755)

	// Setup routes
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/convert", handleConvert)
	http.HandleFunc("/feed", handleRSS)
	http.Handle("/mp3s/", http.StripPrefix("/mp3s/", http.FileServer(http.Dir("mp3s"))))

	fmt.Println("Server starting at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	episodes := getEpisodes()
	data := PageData{Episodes: episodes}
	tmpl.Execute(w, data)
}

func handleConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	youtubeURL := r.FormValue("url")
	if youtubeURL == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Download and convert using youtube-dl
	cmd := exec.Command("yt-dlp",
		"--extract-audio",
		"--audio-format", "mp3",
		"--output", "mp3s/%(title)s.%(ext)s",
		youtubeURL)

	err := cmd.Run()
	if err != nil {
		log.Printf("Error converting video: %v", err)
		http.Redirect(w, r, "/?error=conversion", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/?success=true", http.StatusSeeOther)
}

func handleRSS(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	episodes := getEpisodes()

	rss := `<?xml version="1.0" encoding="UTF-8"?>
    <rss version="2.0"
        xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
        <channel>
            <title>YouTube Downloads</title>
            <link>http://` + host + `</link>
            <description>YouTube videos converted to MP3</description>
            <language>en-us</language>`

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

	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprint(w, rss)
}

func getEpisodes() []Episode {
	var episodes []Episode
	files, _ := ioutil.ReadDir("mp3s")

	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".mp3") {
			// Get audio duration using ffprobe
			duration := getAudioDuration("mp3s/" + f.Name())

			episode := Episode{
				Title:    strings.TrimSuffix(f.Name(), ".mp3"),
				File:     f.Name(),
				PubDate:  f.ModTime().Format(time.RFC1123Z),
				Duration: duration,
				Size:     f.Size(),
			}
			episodes = append(episodes, episode)
		}
	}
	return episodes
}

func getAudioDuration(filepath string) string {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filepath)

	output, err := cmd.Output()
	if err != nil {
		return "Unknown"
	}

	// Convert duration from seconds to HH:MM:SS
	seconds := int(float64(output[0]))
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
}
