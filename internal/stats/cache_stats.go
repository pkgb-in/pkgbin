package stats

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkgb-in/pkgbin/db/repositories"
)

// CacheStats holds cached statistics about the cache directory and database
type CacheStats struct {
	FileCount      int64
	TotalSizeBytes int64
	PackagesServed int64
	LastUpdated    time.Time
	mu             sync.RWMutex
}

// Global instance
var GlobalStats *CacheStats

// InitStats initializes the global stats instance and starts background updates
func InitStats(cacheDir string, updateInterval time.Duration) {
	GlobalStats = &CacheStats{}

	// Initial update
	GlobalStats.updateStats(cacheDir)

	// Start background goroutine for periodic updates
	go func() {
		ticker := time.NewTicker(updateInterval)
		defer ticker.Stop()

		for range ticker.C {
			GlobalStats.updateStats(cacheDir)
		}
	}()

	log.Printf("Cache stats initialized with update interval: %v", updateInterval)
}

// updateStats calculates and updates all statistics
func (s *CacheStats) updateStats(cacheDir string) {
	fileCount, totalSize := calculateCacheStats(cacheDir)
	packagesServed := getTotalPackagesServed()

	s.mu.Lock()
	s.FileCount = fileCount
	s.TotalSizeBytes = totalSize
	s.PackagesServed = packagesServed
	s.LastUpdated = time.Now()
	s.mu.Unlock()

	log.Printf("Stats updated: %d files, %d bytes, %d packages served", fileCount, totalSize, packagesServed)
}

// Get returns the current cached statistics
func (s *CacheStats) Get() (fileCount int64, totalSizeBytes int64, packagesServed int64, lastUpdated time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.FileCount, s.TotalSizeBytes, s.PackagesServed, s.LastUpdated
}

// calculateCacheStats walks the cache directory and calculates file count and total size
func calculateCacheStats(cacheDir string) (fileCount int64, totalSize int64) {
	err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Log error but continue walking
			log.Printf("Error accessing path %s: %v", path, err)
			return nil
		}

		// Only count regular files, not directories
		if !info.IsDir() {
			fileCount++
			totalSize += info.Size()
		}

		return nil
	})

	if err != nil {
		log.Printf("Error walking cache directory %s: %v", cacheDir, err)
	}

	return fileCount, totalSize
}

// getTotalPackagesServed queries the database for total packages served
func getTotalPackagesServed() int64 {
	if repositories.PackageRepo == nil {
		log.Println("PackageRepo is nil, returning 0 for packages served")
		return 0
	}

	total, err := repositories.PackageRepo.GetTotalPackagesServed()
	if err != nil {
		log.Printf("Error getting total packages served: %v", err)
		return 0
	}

	return total
}

// FormatBytes converts bytes to human-readable format
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
