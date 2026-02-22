package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkgb-in/pkgbin/config"
	"github.com/pkgb-in/pkgbin/db/repositories"
	"github.com/pkgb-in/pkgbin/initializers"
	"github.com/pkgb-in/pkgbin/internal/handlers"
	"github.com/pkgb-in/pkgbin/internal/stats"
)

func main() {
	http.HandleFunc("/dashboard", handlers.PyPIDashboardHandler)
	http.HandleFunc("/ping", pingHandler)
	http.HandleFunc("/purge", handlers.PyPIPurgeHandler)
	http.HandleFunc("/refresh-db", handlers.PyPIRefreshHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	if err := initializers.InitDatabase(); err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	repositories.InitPackageRepository()

	// Initialize cache statistics with 5-minute update interval
	stats.InitStats(config.PyPIConfig.CacheDir, 5*time.Minute)

	ListenHost := config.Server.Host
	ListenPort := config.Server.Port
	CacheDir := config.PyPIConfig.CacheDir
	Upstream := config.PyPIConfig.Upstream

	_ = os.MkdirAll(CacheDir, 0755)

	target, _ := url.Parse(Upstream)
	proxy := httputil.NewSingleHostReverseProxy(target)

	// The Director ensures the outgoing request has the correct Host header
	// for PyPI. We preserve the original host to use in URL rewriting.
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		// Store the original Host header before modifying it
		originalHost := req.Host
		if originalHost == "" {
			originalHost = req.URL.Host
		}
		// Store in a custom header so we can access it in ModifyResponse
		req.Header.Set("X-Original-Host", originalHost)

		originalDirector(req)
		req.Host = target.Host
	}

	// Modify the response to rewrite CDN URLs to point to our proxy
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Only process Simple API responses
		if !strings.Contains(resp.Request.URL.Path, "/simple/") {
			return nil
		}

		contentType := resp.Header.Get("Content-Type")
		// Only process JSON and HTML responses
		if !strings.Contains(contentType, "json") && !strings.Contains(contentType, "html") {
			return nil
		}

		// Get the original client host
		originalHost := resp.Request.Header.Get("X-Original-Host")
		if originalHost == "" {
			originalHost = resp.Request.Host
		}

		// Read the response body
		var body []byte
		var err error

		// Handle gzip encoding
		if resp.Header.Get("Content-Encoding") == "gzip" {
			gr, err := gzip.NewReader(resp.Body)
			if err != nil {
				log.Printf("ERROR: Failed to create gzip reader: %v", err)
				return nil
			}
			defer gr.Close()
			body, err = io.ReadAll(gr)
			if err != nil {
				log.Printf("ERROR: Failed to read gzip body: %v", err)
				return nil
			}
			resp.Header.Del("Content-Encoding")
		} else {
			body, err = io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("ERROR: Failed to read response body: %v", err)
				return nil
			}
		}
		resp.Body.Close()

		// Replace CDN URLs with our proxy
		proxyURL := "http://" + originalHost
		modifiedBody := bytes.ReplaceAll(body, []byte("https://files.pythonhosted.org"), []byte(proxyURL))

		// Set the new body
		resp.Body = io.NopCloser(bytes.NewReader(modifiedBody))
		resp.ContentLength = int64(len(modifiedBody))
		resp.Header.Set("Content-Length", strconv.FormatInt(int64(len(modifiedBody)), 10))
		resp.Header.Del("Transfer-Encoding")

		if bytes.Contains(body, []byte("files.pythonhosted.org")) {
			log.Printf("Rewrote PyPI URLs for %s (size: %d bytes)", resp.Request.URL.Path, len(modifiedBody))
		}
		return nil
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)

		// 1. Intercept GET requests for package files (.whl, .tar.gz, .zip, .egg)
		if r.Method == http.MethodGet && isPackageFile(r.URL.Path) {
			handlers.PyPIDownloadHandler(w, r)
			return
		}

		// 2. Forward everything else (simple API, JSON API, metadata, etc.)
		proxy.ServeHTTP(w, r)
	})

	log.Printf("PyPI Proxy started on :8080")
	log.Fatal(http.ListenAndServe(ListenHost+":"+ListenPort, nil))
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"pong"}`))
}

// isPackageFile checks if the URL path points to a Python package file
func isPackageFile(path string) bool {
	lowerPath := strings.ToLower(path)
	return strings.HasSuffix(lowerPath, ".whl") ||
		strings.HasSuffix(lowerPath, ".tar.gz") ||
		strings.HasSuffix(lowerPath, ".zip") ||
		strings.HasSuffix(lowerPath, ".egg") ||
		strings.HasSuffix(lowerPath, ".tar.bz2")
}
