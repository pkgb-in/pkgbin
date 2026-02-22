package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pkgb-in/pkgbin/config"
	"github.com/pkgb-in/pkgbin/db/repositories"
	"github.com/pkgb-in/pkgbin/initializers"
	"github.com/pkgb-in/pkgbin/internal/handlers"
	"github.com/pkgb-in/pkgbin/internal/stats"
)

func main() {
	http.HandleFunc("/dashboard", handlers.NPMDashboardHandler)
	http.HandleFunc("/ping", pingHandler)
	http.HandleFunc("/purge", handlers.NPMPurgeHandler)
	http.HandleFunc("/refresh-db", handlers.NPMRefreshHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	if err := initializers.InitDatabase(); err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	repositories.InitPackageRepository()

	// Initialize cache statistics with 5-minute update interval
	stats.InitStats(config.NPMConfig.CacheDir, 5*time.Minute)

	ListenHost := config.Server.Host
	ListenPort := config.Server.Port
	CacheDir := config.NPMConfig.CacheDir
	Upstream := config.NPMConfig.Upstream
	ProxyAddr := "http://" + config.Server.Host + ":" + config.Server.Port

	_ = os.MkdirAll(CacheDir, 0755)

	target, _ := url.Parse(Upstream)
	proxy := httputil.NewSingleHostReverseProxy(target)

	// The Director ensures the outgoing request has the correct Host header
	// for the official NPM registry.
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
	}

	// Modify the response for metadata (JSON) to rewrite URLs to this proxy
	proxy.ModifyResponse = func(resp *http.Response) error {
		if r := resp.Request; r != nil && !strings.HasSuffix(r.URL.Path, ".tgz") {
			// Only rewrite if it's likely a JSON metadata response
			if strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
				body, _ := io.ReadAll(resp.Body)
				newBody := bytes.ReplaceAll(body, []byte(Upstream), []byte(ProxyAddr))
				resp.Body = io.NopCloser(bytes.NewReader(newBody))
				resp.ContentLength = int64(len(newBody))
			}
		}
		return nil
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)

		// 1. Intercept GET requests for tarballs to handle caching
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, ".tgz") {
			handlers.HandleTarballDownload(w, r)
			return
		}

		// 2. Forward everything else (POST audits, Metadata, etc.)
		proxy.ServeHTTP(w, r)
	})

	log.Printf("NPM Proxy started on :8080")
	log.Fatal(http.ListenAndServe(ListenHost+":"+ListenPort, nil))

}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"pong"}`))
}
