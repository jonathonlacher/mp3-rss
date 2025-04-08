package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
)

//go:embed static
var staticFiles embed.FS

//go:embed templates
var templateFiles embed.FS

// setupStaticFiles sets up handlers for static files embedded in the binary
func setupStaticFiles() {
	// Create a sub-filesystem for static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("Failed to create sub-filesystem for static files: %v", err)
	}

	// Serve static files from the embedded filesystem
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
}
