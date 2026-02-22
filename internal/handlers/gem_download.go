package handlers

import (
	"crypto/sha512"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkgb-in/pkgbin/config"
	"github.com/pkgb-in/pkgbin/db/repositories"
)

// gemDownloadLocks prevents concurrent downloads of the same gem
var gemDownloadLocks = make(map[string]*sync.Mutex)
var gemDownloadLocksMutex sync.Mutex

func GemDownloadHandler(w http.ResponseWriter, r *http.Request) {

	Upstream := config.RubyGemsConfig.Upstream
	CacheDir := config.RubyGemsConfig.CacheDir

	gemFileName := filepath.Base(r.URL.Path)
	localPath := filepath.Join(CacheDir, gemFileName)

	// Check local cache and verify integrity
	if stat, err := os.Stat(localPath); err == nil && stat.Size() > 0 {
		// Verify file is readable before serving
		if file, err := os.Open(localPath); err == nil {
			file.Close()
			log.Printf("Serving from cache: %s", gemFileName)
			repositories.PackageRepo.UpdatePackageAccess(gemFileName, true)
			http.ServeFile(w, r, localPath)
			return
		} else {
			// File exists but can't be read - delete it
			log.Printf("Corrupted cache file detected, removing: %s", gemFileName)
			os.Remove(localPath)
		}
	}

	// Get or create a lock for this specific file to prevent concurrent downloads
	gemDownloadLocksMutex.Lock()
	lock, exists := gemDownloadLocks[gemFileName]
	if !exists {
		lock = &sync.Mutex{}
		gemDownloadLocks[gemFileName] = lock
	}
	gemDownloadLocksMutex.Unlock()

	// Lock this specific file download
	lock.Lock()
	defer lock.Unlock()

	// Double-check cache after acquiring lock (another request may have downloaded it)
	if stat, err := os.Stat(localPath); err == nil && stat.Size() > 0 {
		if file, err := os.Open(localPath); err == nil {
			file.Close()
			log.Printf("Serving from cache (after lock): %s", gemFileName)
			repositories.PackageRepo.UpdatePackageAccess(gemFileName, true)
			http.ServeFile(w, r, localPath)
			return
		}
	}

	// Not in cache, fetch from upstream
	log.Printf("Cache miss. Fetching from upstream: %s", gemFileName)
	repositories.PackageRepo.UpdatePackageAccess(gemFileName, false)
	upstreamURL := Upstream + r.URL.Path

	// Use a client that handles redirects properly (stripping headers for S3)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 0 && req.URL.Host != via[0].URL.Host {
				req.Header.Del("Authorization")
			}
			return nil
		},
	}

	resp, err := client.Get(upstreamURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		http.Error(w, "Failed to fetch gem from upstream", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

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
		log.Printf("Download error for %s: %v", gemFileName, err)
		return
	}

	// Verify file was written completely
	if stat, err := os.Stat(tempPath); err != nil || stat.Size() != bytesWritten {
		os.Remove(tempPath)
		http.Error(w, "File write verification failed", http.StatusInternalServerError)
		log.Printf("Size mismatch for %s: expected %d, got %d", gemFileName, bytesWritten, stat.Size())
		return
	}

	// Atomically move temp file to final location
	if err := os.Rename(tempPath, localPath); err != nil {
		os.Remove(tempPath)
		http.Error(w, "File move failed", http.StatusInternalServerError)
		log.Printf("Failed to move temp file for %s: %v", gemFileName, err)
		return
	}

	// Log the file hash for debugging
	fileHash := hex.EncodeToString(hash.Sum(nil))
	log.Printf("Cached %s (size: %d bytes, sha512: %s)", gemFileName, bytesWritten, fileHash[:16]+"...")

	// Serve the newly cached file
	http.ServeFile(w, r, localPath)
}
