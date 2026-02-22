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

// downloadLocks prevents concurrent downloads of the same file
var downloadLocks = make(map[string]*sync.Mutex)
var downloadLocksMutex sync.Mutex

// generateCacheFileName creates a unique filename from npm URL path
// Handles scoped packages like @types/package-name
func generateCacheFileName(urlPath string) string {
	// Remove leading slash
	urlPath = strings.TrimPrefix(urlPath, "/")

	// For scoped packages like /@types/html-minifier-terser/-/html-minifier-terser-6.1.0.tgz
	// Extract scope and tarball name
	if strings.HasPrefix(urlPath, "@") {
		parts := strings.Split(urlPath, "/-/")
		if len(parts) == 2 {
			scope := strings.TrimPrefix(parts[0], "@")
			scope = strings.ReplaceAll(scope, "/", "__")
			tarballName := filepath.Base(parts[1])
			return "@" + scope + "__" + tarballName
		}
	}

	// For regular packages, just use the tarball name
	return filepath.Base(urlPath)
}

// func HandleMetadata(w http.ResponseWriter, r *http.Request) {

// 	Upstream := config.NPMConfig.Upstream
// 	ProxyAddr := "http://" + config.Server.Host + ":" + config.Server.Port

// 	upstreamURL := Upstream + r.URL.Path
// 	resp, err := http.Get(upstreamURL)
// 	if err != nil {
// 		http.Error(w, "Upstream error", http.StatusBadGateway)
// 		return
// 	}
// 	defer resp.Body.Close()

// 	// Read the JSON body so we can modify it
// 	body, _ := io.ReadAll(resp.Body)

// 	// REWRITE logic: Replace registry.npmjs.org with our local proxy address
// 	// This forces the NPM client to request the .tgz from US.
// 	modifiedBody := bytes.ReplaceAll(body, []byte(Upstream), []byte(ProxyAddr))

// 	w.Header().Set("Content-Type", "application/json")
// 	w.Write(modifiedBody)
// }

// func HandleTarballDownload(w http.ResponseWriter, r *http.Request) {

// 	Upstream := config.NPMConfig.Upstream
// 	CacheDir := config.NPMConfig.CacheDir

// 	fileName := filepath.Base(r.URL.Path)
// 	localPath := filepath.Join(CacheDir, fileName)

// 	// Check Cache
// 	if _, err := os.Stat(localPath); err == nil {
// 		log.Printf("NPM Cache Hit: %s", fileName)
// 		http.ServeFile(w, r, localPath)
// 		return
// 	}

// 	// Fetch from Upstream
// 	log.Printf("NPM Cache Miss: %s", fileName)
// 	upstreamURL := Upstream + r.URL.Path
// 	resp, err := http.Get(upstreamURL)
// 	if err != nil || resp.StatusCode != 200 {
// 		http.Error(w, "Failed to fetch tarball", http.StatusBadGateway)
// 		return
// 	}
// 	defer resp.Body.Close()

// 	// Save to Cache
// 	outFile, _ := os.Create(localPath)
// 	defer outFile.Close()

// 	tee := io.TeeReader(resp.Body, outFile)
// 	io.Copy(w, tee)
// }

func HandleTarballDownload(w http.ResponseWriter, r *http.Request) {

	Upstream := config.NPMConfig.Upstream
	CacheDir := config.NPMConfig.CacheDir

	// Extract unique filename preserving scoped packages
	// e.g., /@types/html-minifier-terser/-/html-minifier-terser-6.1.0.tgz
	// becomes: @types__html-minifier-terser-6.1.0.tgz
	fileName := generateCacheFileName(r.URL.Path)
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
	downloadLocksMutex.Lock()
	lock, exists := downloadLocks[fileName]
	if !exists {
		lock = &sync.Mutex{}
		downloadLocks[fileName] = lock
	}
	downloadLocksMutex.Unlock()

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
	log.Printf("Cache miss: Fetching %s", fileName)
	repositories.PackageRepo.UpdatePackageAccess(fileName, false)
	resp, err := http.Get(Upstream + r.URL.Path)
	if err != nil || resp.StatusCode != http.StatusOK {
		http.Error(w, "Upstream fetch failed", http.StatusBadGateway)
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
