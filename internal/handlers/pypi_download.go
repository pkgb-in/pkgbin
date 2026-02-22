package handlers

import (
	"crypto/sha512"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkgb-in/pkgbin/config"
	"github.com/pkgb-in/pkgbin/db/repositories"
)

// pypiDownloadLocks prevents concurrent downloads of the same package
var pypiDownloadLocks = make(map[string]*sync.Mutex)
var pypiDownloadLocksMutex sync.Mutex

// generatePyPICacheFileName creates a unique filename from PyPI URL path
// PyPI URLs can be complex: /packages/source/p/package/package-1.0.0.tar.gz
// or /packages/py3/p/package/package-1.0.0-py3-none-any.whl
// We preserve the structure by replacing slashes with double underscores
func generatePyPICacheFileName(urlPath string) string {
	// Remove leading slash
	urlPath = strings.TrimPrefix(urlPath, "/")

	// For PyPI packages like /packages/source/p/package/package-1.0.0.tar.gz
	// Convert to: packages__source__p__package__package-1.0.0.tar.gz
	// This ensures uniqueness across different package structures

	// Replace all slashes except the last one (filename)
	parts := strings.Split(urlPath, "/")
	if len(parts) > 1 {
		// Join all directory parts with __ and keep the filename
		dirParts := parts[:len(parts)-1]
		fileName := parts[len(parts)-1]
		return strings.Join(dirParts, "__") + "__" + fileName
	}

	// Fallback to just the filename
	return filepath.Base(urlPath)
}

func PyPIDownloadHandler(w http.ResponseWriter, r *http.Request) {

	Upstream := config.PyPIConfig.Upstream
	CacheDir := config.PyPIConfig.CacheDir

	// Generate unique cache filename preserving PyPI structure
	fileName := generatePyPICacheFileName(r.URL.Path)
	localPath := filepath.Join(CacheDir, fileName)

	// Check local cache and verify integrity
	if stat, err := os.Stat(localPath); err == nil && stat.Size() > 0 {
		// Verify file is readable before serving
		if file, err := os.Open(localPath); err == nil {
			file.Close()
			log.Printf("Serving from cache: %s", fileName)
			repositories.PackageRepo.UpdatePackageAccess(fileName, true)
			http.ServeFile(w, r, localPath)
			return
		} else {
			// File exists but can't be read - delete it
			log.Printf("Corrupted cache file detected, removing: %s", fileName)
			os.Remove(localPath)
		}
	}

	// Get or create a lock for this specific file to prevent concurrent downloads
	pypiDownloadLocksMutex.Lock()
	lock, exists := pypiDownloadLocks[fileName]
	if !exists {
		lock = &sync.Mutex{}
		pypiDownloadLocks[fileName] = lock
	}
	pypiDownloadLocksMutex.Unlock()

	// Lock this specific file download
	lock.Lock()
	defer lock.Unlock()

	// Double-check cache after acquiring lock (another request may have downloaded it)
	if stat, err := os.Stat(localPath); err == nil && stat.Size() > 0 {
		if file, err := os.Open(localPath); err == nil {
			file.Close()
			log.Printf("Serving from cache (after lock): %s", fileName)
			repositories.PackageRepo.UpdatePackageAccess(fileName, true)
			http.ServeFile(w, r, localPath)
			return
		}
	}

	// Cache miss: Fetch from upstream
	log.Printf("Cache miss: Fetching %s from %s", fileName, r.URL.Path)
	repositories.PackageRepo.UpdatePackageAccess(fileName, false)

	// PyPI packages are hosted on files.pythonhosted.org CDN
	// The URL path contains the full package location
	var upstreamURL string
	if strings.HasPrefix(r.URL.Path, "/packages/") {
		// Direct package file request - use CDN
		upstreamURL = "https://files.pythonhosted.org" + r.URL.Path
	} else {
		// Fallback to main PyPI
		upstreamURL = Upstream + r.URL.Path
	}

	log.Printf("Fetching from upstream: %s", upstreamURL)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Follow redirects to CDN
			return nil
		},
	}

	resp, err := client.Get(upstreamURL)
	if err != nil {
		http.Error(w, "Upstream fetch failed", http.StatusBadGateway)
		log.Printf("Failed to fetch from upstream: %s (error: %v)", upstreamURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Upstream fetch failed", http.StatusBadGateway)
		log.Printf("Failed to fetch from upstream: %s (status: %d)", upstreamURL, resp.StatusCode)
		return
	}

	// Use temporary file for atomic write
	tempPath := localPath + ".tmp"
	outFile, err := os.Create(tempPath)
	if err != nil {
		http.Error(w, "File creation failed", http.StatusInternalServerError)
		return
	}

	// Download completely to temp file first with integrity check
	hash := sha512.New()
	multiWriter := io.MultiWriter(outFile, hash)
	bytesWritten, err := io.Copy(multiWriter, resp.Body)
	outFile.Close()

	if err != nil {
		os.Remove(tempPath)
		http.Error(w, "Download failed", http.StatusInternalServerError)
		log.Printf("Download error for %s: %v", fileName, err)
		return
	}

	// Verify file was written completely
	if stat, err := os.Stat(tempPath); err != nil || stat.Size() != bytesWritten {
		os.Remove(tempPath)
		http.Error(w, "File write verification failed", http.StatusInternalServerError)
		log.Printf("Size mismatch for %s: expected %d, got %d", fileName, bytesWritten, stat.Size())
		return
	}

	// Atomically move temp file to final location
	if err := os.Rename(tempPath, localPath); err != nil {
		os.Remove(tempPath)
		http.Error(w, "File move failed", http.StatusInternalServerError)
		log.Printf("Failed to move temp file for %s: %v", fileName, err)
		return
	}

	// Log the file hash for debugging
	fileHash := hex.EncodeToString(hash.Sum(nil))
	log.Printf("Cached %s (size: %d bytes, sha512: %s)", fileName, bytesWritten, fileHash[:16]+"...")

	// Serve the newly cached file
	http.ServeFile(w, r, localPath)
}
