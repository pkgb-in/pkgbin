package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pkgb-in/pkgbin/config"
	"github.com/pkgb-in/pkgbin/db/repositories"
)

type PurgeRequest struct {
	Packages []string `json:"packages"`
}

type PurgeResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	Deleted []string `json:"deleted,omitempty"`
	Failed  []string `json:"failed,omitempty"`
}

func NPMPurgeHandler(w http.ResponseWriter, r *http.Request) {
	purgeHandler(w, r, config.NPMConfig.CacheDir, "npm")
}

func RubyPurgeHandler(w http.ResponseWriter, r *http.Request) {
	purgeHandler(w, r, config.RubyGemsConfig.CacheDir, "gem")
}

func PyPIPurgeHandler(w http.ResponseWriter, r *http.Request) {
	purgeHandler(w, r, config.PyPIConfig.CacheDir, "pypi")
}

func purgeHandler(w http.ResponseWriter, r *http.Request, cacheDir, packageType string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PurgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Packages) == 0 {
		json.NewEncoder(w).Encode(PurgeResponse{
			Success: true,
			Message: "No packages to purge",
		})
		return
	}

	deleted := []string{}
	failed := []string{}

	// Delete from cache directory
	for _, pkgName := range req.Packages {
		if packageType == "npm" {
			// NPM packages are stored as tarballs: package-version.tgz
			// We need to find all files matching the package name pattern
			pattern := filepath.Join(cacheDir, pkgName)
			matches, err := filepath.Glob(pattern)
			if err != nil {
				log.Printf("Error finding NPM cache files for %s: %v", pkgName, err)
				failed = append(failed, pkgName)
				continue
			}

			deletedFiles := false
			for _, match := range matches {
				if err := os.Remove(match); err != nil {
					log.Printf("Error deleting NPM cache file %s: %v", match, err)
				} else {
					log.Printf("Deleted NPM cache file: %s", match)
					deletedFiles = true
				}
			}

			if !deletedFiles && len(matches) == 0 {
				log.Printf("No NPM cache files found for package: %s", pkgName)
			}
		} else {
			// Ruby gems are stored as: package-version.gem
			pattern := filepath.Join(cacheDir, pkgName)
			matches, err := filepath.Glob(pattern)
			if err != nil {
				log.Printf("Error finding gem cache files for %s: %v", pkgName, err)
				failed = append(failed, pkgName)
				continue
			}

			deletedFiles := false
			for _, match := range matches {
				if err := os.Remove(match); err != nil {
					log.Printf("Error deleting gem cache file %s: %v", match, err)
				} else {
					log.Printf("Deleted gem cache file: %s", match)
					deletedFiles = true
				}
			}

			if !deletedFiles && len(matches) == 0 {
				log.Printf("No gem cache files found for package: %s", pkgName)
			}
		}
	}

	// Delete from database
	if err := repositories.PackageRepo.DeletePackagesByNames(req.Packages); err != nil {
		log.Printf("Error deleting packages from database: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PurgeResponse{
			Success: false,
			Message: "Failed to delete packages from database",
		})
		return
	}

	deleted = req.Packages
	log.Printf("Successfully purged %d packages", len(deleted))

	w.Header().Set("Content-Type", "application/json")
	response := PurgeResponse{
		Success: true,
		Message: "Packages purged successfully",
		Deleted: deleted,
	}

	if len(failed) > 0 {
		response.Failed = failed
		response.Message = "Some packages failed to purge completely"
	}

	json.NewEncoder(w).Encode(response)
}
