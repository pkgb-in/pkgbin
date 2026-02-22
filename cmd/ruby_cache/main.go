package main

import (
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
	http.HandleFunc("/dashboard", handlers.RubyDashboardHandler)
	http.HandleFunc("/ping", pingHandler)
	http.HandleFunc("/purge", handlers.RubyPurgeHandler)
	http.HandleFunc("/refresh-db", handlers.RubyRefreshHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	if err := initializers.InitDatabase(); err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	repositories.InitPackageRepository()

	// Initialize cache statistics with 5-minute update interval
	stats.InitStats(config.RubyGemsConfig.CacheDir, 5*time.Minute)

	ListenHost := config.Server.Host
	ListenPort := config.Server.Port

	Upstream := config.RubyGemsConfig.Upstream
	CacheDir := config.RubyGemsConfig.CacheDir

	_ = os.MkdirAll(CacheDir, 0755)

	target, _ := url.Parse(Upstream)
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Custom Director to ensure Host header is set correctly for RubyGems/S3
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 1. Handle Gem Downloads (The Caching Part)
		if strings.HasPrefix(r.URL.Path, "/gems/") && strings.HasSuffix(r.URL.Path, ".gem") {
			handlers.GemDownloadHandler(w, r)
			return
		}

		// 2. Relay everything else (API calls, specs, etc.)
		log.Printf("Proxying metadata request: %s", r.URL.Path)
		proxy.ServeHTTP(w, r)
	})

	log.Printf("RubyGems Proxy started on %s", ListenPort)
	log.Fatal(http.ListenAndServe(ListenHost+":"+ListenPort, nil))
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"pong"}`))
}
