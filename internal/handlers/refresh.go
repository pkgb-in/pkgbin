package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkgb-in/pkgbin/db/models"
	"github.com/pkgb-in/pkgbin/db/repositories"
)

var (
	lastRefreshTime   time.Time
	refreshMutex      sync.Mutex
	refreshInProgress bool
)

type RefreshResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func NPMRefreshHandler(w http.ResponseWriter, r *http.Request) {
	refreshHandler(w, r, "./npm_cache_data")
}

func RubyRefreshHandler(w http.ResponseWriter, r *http.Request) {
	refreshHandler(w, r, "./gem_cache_data")
}

func PyPIRefreshHandler(w http.ResponseWriter, r *http.Request) {
	refreshHandler(w, r, "./pypi_cache_data")
}

func refreshHandler(w http.ResponseWriter, r *http.Request, cacheDir string) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(RefreshResponse{
			Success: false,
			Message: "Method not allowed",
		})
		return
	}

	refreshMutex.Lock()

	// Check if a refresh is already in progress
	if refreshInProgress {
		refreshMutex.Unlock()
		json.NewEncoder(w).Encode(RefreshResponse{
			Success: false,
			Message: "A refresh operation is already in progress. Please wait.",
		})
		return
	}

	// Check if last refresh was within 30 minutes
	timeSinceLastRefresh := time.Since(lastRefreshTime)
	if timeSinceLastRefresh < 30*time.Minute && !lastRefreshTime.IsZero() {
		refreshMutex.Unlock()
		remainingTime := 30*time.Minute - timeSinceLastRefresh
		json.NewEncoder(w).Encode(RefreshResponse{
			Success: false,
			Message: "Please wait " + remainingTime.Round(time.Minute).String() + " before refreshing again.",
		})
		return
	}

	// Mark refresh as in progress
	refreshInProgress = true
	lastRefreshTime = time.Now()
	refreshMutex.Unlock()

	// Start background job
	go performDatabaseRefresh(cacheDir)

	json.NewEncoder(w).Encode(RefreshResponse{
		Success: true,
		Message: "Database refresh started in background. This may take a few minutes.",
	})
}

func performDatabaseRefresh(cacheDir string) {
	defer func() {
		refreshMutex.Lock()
		refreshInProgress = false
		refreshMutex.Unlock()
	}()

	log.Println("Starting database refresh operation...")

	// Step 1: Truncate packages table
	if err := repositories.PackageRepo.TruncatePackagesTable(); err != nil {
		log.Printf("Error truncating packages table: %v", err)
		return
	}
	log.Println("Packages table truncated")

	// Step 2: Reset ID sequence
	if err := repositories.PackageRepo.ResetSequence(); err != nil {
		log.Printf("Error resetting sequence: %v", err)
		return
	}
	log.Println("ID sequence reset")

	// Step 3: Scan cache directory and add packages
	packageCount := 0
	err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Error accessing path %s: %v", path, err)
			return nil
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get just the filename
		filename := filepath.Base(path)

		// Create package entry with initial stats
		pkg := models.Package{
			Name:      filename,
			CacheHit:  0,
			CacheMiss: 0,
		}

		if err := repositories.PackageRepo.CreatePackage(&pkg); err != nil {
			log.Printf("Error creating package entry for %s: %v", filename, err)
			return nil
		}

		packageCount++
		if packageCount%100 == 0 {
			log.Printf("Processed %d packages...", packageCount)
		}

		return nil
	})

	if err != nil {
		log.Printf("Error scanning cache directory: %v", err)
		return
	}

	log.Printf("Database refresh completed. Added %d packages to database.", packageCount)
}
