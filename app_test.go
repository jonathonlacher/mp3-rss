package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewApp tests the NewApp constructor function
func TestNewApp(t *testing.T) {
	// Create a test config
	config := AppConfig{
		MP3Dir: "/test/mp3/dir",
	}

	// Create the app
	app := NewApp(config)

	// Verify the app was created correctly
	if app == nil {
		t.Fatal("NewApp returned nil")
	}

	if app.config.MP3Dir != config.MP3Dir {
		t.Errorf("expected MP3Dir to be %q, got %q", config.MP3Dir, app.config.MP3Dir)
	}

	if app.progressMap == nil {
		t.Error("expected progressMap to be initialized, got nil")
	}
}

// TestSanitizeFilename tests the sanitizeFilename function
func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal string",
			input:    "normal string",
			expected: "normal string",
		},
		{
			name:     "String with invalid characters",
			input:    "file/with:invalid*chars?",
			expected: "file-with-invalid-chars-",
		},
		{
			name:     "String with all special characters",
			input:    `/\:*?"<>|`,
			expected: "--------",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q",
					tt.input, result, tt.expected)
			}
		})
	}
}

// TestEscapeXML tests the escapeXML function
func TestEscapeXML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal string",
			input:    "normal string",
			expected: "normal string",
		},
		{
			name:     "String with XML special characters",
			input:    "text with <tags> & \"quotes\"",
			expected: "text with &lt;tags&gt; &amp; \"quotes\"",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeXML(tt.input)
			if result != tt.expected {
				t.Errorf("escapeXML(%q) = %q, want %q",
					tt.input, result, tt.expected)
			}
		})
	}
}

// createTempDir creates a temporary directory for tests
func createTempDir(t *testing.T) string {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "mp3-rss-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	t.Cleanup(func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Error removing temp directory: %v", err)
		}
	})

	return tempDir
}

// createTestApp creates an App instance for testing with a temporary directory
func createTestApp(t *testing.T) (*App, string) {
	t.Helper()

	// Create a temporary directory for MP3 files
	tempDir := createTempDir(t)

	// Create an App with the temp directory as MP3Dir
	config := AppConfig{
		MP3Dir: tempDir,
	}
	app := NewApp(config)

	return app, tempDir
}

// TestGetEpisodes tests the getEpisodes method
func TestGetEpisodes(t *testing.T) {
	// Create a test app with a temporary directory
	app, tempDir := createTestApp(t)

	// Create some test MP3 files
	testFiles := []string{
		"test1.mp3",
		"test2_NORM_20250101.mp3",
		"test3.mp3",
	}

	for _, file := range testFiles {
		filePath := filepath.Join(tempDir, file)
		if err := os.WriteFile(filePath, []byte("test data"), 0644); err != nil {
			t.Fatalf("Failed to create test file %q: %v", file, err)
		}
	}

	// Get the episodes
	episodes := app.getEpisodes()

	// Verify the correct number of episodes was returned
	if len(episodes) != len(testFiles) {
		t.Errorf("Expected %d episodes, got %d", len(testFiles), len(episodes))
	}

	// Verify normalized flag is correctly set
	for _, episode := range episodes {
		if episode.File == "test2_NORM_20250101.mp3" && !episode.IsNormalized {
			t.Error("Expected test2_NORM_20250101.mp3 to have IsNormalized=true")
		} else if episode.File == "test1.mp3" && episode.IsNormalized {
			t.Error("Expected test1.mp3 to have IsNormalized=false")
		}
	}
}

// TestDeleteEpisode tests the deleteEpisode method
func TestDeleteEpisode(t *testing.T) {
	// Create a test app with a temporary directory
	app, tempDir := createTestApp(t)

	// Create a test MP3 file
	testFile := "test_delete.mp3"
	filePath := filepath.Join(tempDir, testFile)
	if err := os.WriteFile(filePath, []byte("test data"), 0644); err != nil {
		t.Fatalf("Failed to create test file %q: %v", testFile, err)
	}

	// Test deleting a valid file
	err := app.deleteEpisode(testFile)
	if err != nil {
		t.Errorf("deleteEpisode(%q) returned error: %v", testFile, err)
	}

	// Verify the file was deleted
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("Expected file %q to be deleted, but it still exists", filePath)
	}

	// Test deleting a non-existent file
	err = app.deleteEpisode("nonexistent.mp3")
	if err == nil {
		t.Error("Expected error when deleting non-existent file, got nil")
	}
}
