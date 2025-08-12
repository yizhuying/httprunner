package uixt

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// DetectAndRenameImageFile examines the file content to determine its image type
// and renames the file with the appropriate extension (.jpg, .png, etc.)
func DetectAndRenameImageFile(filePath string) (string, error) {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for type detection: %v", err)
	}
	defer file.Close()

	// Read the first 512 bytes to detect content type
	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read file for type detection: %v", err)
	}

	// Reset file pointer
	_, err = file.Seek(0, 0)
	if err != nil {
		return "", fmt.Errorf("failed to reset file pointer: %v", err)
	}

	// Detect content type
	contentType := http.DetectContentType(buffer)
	log.Info().Str("filePath", filePath).Str("contentType", contentType).Msg("Detected content type")

	// Determine file extension based on content type
	var extension string
	switch {
	case strings.Contains(contentType, "image/jpeg"):
		extension = ".jpg"
	case strings.Contains(contentType, "image/png"):
		extension = ".png"
	case strings.Contains(contentType, "image/gif"):
		extension = ".gif"
	case strings.Contains(contentType, "image/webp"):
		extension = ".webp"
	case strings.Contains(contentType, "image/bmp"):
		extension = ".bmp"
	case strings.Contains(contentType, "image/tiff"):
		extension = ".tiff"
	case strings.Contains(contentType, "image/svg+xml"):
		extension = ".svg"
	default:
		// Default to jpg if we can't determine the type but it's still an image
		if strings.Contains(contentType, "image/") {
			extension = ".jpg"
		} else {
			return filePath, fmt.Errorf("not a recognized image type: %s", contentType)
		}
	}

	// Create new file path with extension
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)
	newFilePath := filepath.Join(dir, base+extension)

	// If the file already has the correct extension, just return it
	if filePath == newFilePath {
		return filePath, nil
	}

	// Rename the file
	err = os.Rename(filePath, newFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to rename file: %v", err)
	}

	log.Info().Str("oldPath", filePath).Str("newPath", newFilePath).Msg("Renamed image file with proper extension")
	return newFilePath, nil
}
