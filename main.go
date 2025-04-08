package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("Starting MP3-RSS server...")

	// Setup directories
	mp3Dir, err := filepath.Abs("mp3s")
	if err != nil {
		log.Fatalf("Failed to resolve absolute path for mp3s directory: %v", err)
	}

	log.Printf("Using MP3 directory: %s", mp3Dir)

	if err := os.MkdirAll(mp3Dir, 0755); err != nil {
		log.Fatalf("Failed to create mp3s directory %q: %v", mp3Dir, err)
	}

	// Verify the directory is accessible and has write permissions
	testFile := filepath.Join(mp3Dir, ".write-test")
	f, err := os.Create(testFile)
	if err != nil {
		log.Fatalf("Cannot write to mp3s directory %q: %v", mp3Dir, err)
	}
	if err := f.Close(); err != nil {
		log.Printf("Warning: Error closing test file: %v", err)
	}
	if err := os.Remove(testFile); err != nil {
		log.Printf("Warning: Error removing test file: %v", err)
	}

	// Make sure required executables exist
	if err := checkRequiredExecutables(); err != nil {
		log.Fatalf("Missing required executables: %v", err)
	}

	// Create the application with configuration
	app := NewApp(AppConfig{
		MP3Dir: mp3Dir,
	})

	// Set up HTTP routes
	app.SetupRoutes()

	// Start the server
	address := ":8080"
	log.Printf("Server starting on http://localhost%s", address)
	err = http.ListenAndServe(address, nil)
	if err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// checkRequiredExecutables verifies that required external programs are installed
func checkRequiredExecutables() error {
	required := []string{"yt-dlp", "ffmpeg", "ffprobe"}

	for _, cmd := range required {
		if err := checkExecutableExists(cmd); err != nil {
			return fmt.Errorf("%s not found in PATH: %w", cmd, err)
		}
	}

	return nil
}

// checkExecutableExists verifies that a command exists in the PATH
func checkExecutableExists(name string) error {
	_, err := exec.LookPath(name)
	return err
}
