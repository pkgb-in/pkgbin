package handlers

import (
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/pkgb-in/pkgbin/db/models"
	"github.com/pkgb-in/pkgbin/db/repositories"
	"github.com/pkgb-in/pkgbin/internal/stats"
)

type DashboardPackage struct {
	Name      string
	CacheHit  int64
	CacheMiss int64
}

type DashboardData struct {
	Title          string
	Packages       []DashboardPackage
	CurrentPage    int
	TotalPages     int
	FileCount      int64
	CacheSize      string
	PackagesServed int64
	LastUpdated    string
}

func NPMDashboardHandler(w http.ResponseWriter, r *http.Request) {
	dashboardHandler(w, r, "Package Bin for NPM")
}

func RubyDashboardHandler(w http.ResponseWriter, r *http.Request) {
	dashboardHandler(w, r, "Package Bin for RubyGems")
}

func PyPIDashboardHandler(w http.ResponseWriter, r *http.Request) {
	dashboardHandler(w, r, "Package Bin for PyPI")
}

func DashboardHandler(w http.ResponseWriter, r *http.Request) {
	dashboardHandler(w, r, "Package Dashboard")
}

func dashboardHandler(w http.ResponseWriter, r *http.Request, title string) {
	const pageSize = 20
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}

	filter := r.URL.Query().Get("filter")
	var pkgs []models.Package
	var total int
	var err error
	if filter != "" {
		pkgs, total, err = repositories.PackageRepo.ListPackagesByNamePaginated(filter, page, pageSize)
	} else {
		pkgs, total, err = repositories.PackageRepo.ListPackagesPaginated(page, pageSize)
	}
	if err != nil {
		http.Error(w, "Failed to load packages", http.StatusInternalServerError)
		return
	}

	var dashPkgs []DashboardPackage
	for _, pkg := range pkgs {
		dashPkgs = append(dashPkgs, DashboardPackage{
			Name:      pkg.Name,
			CacheHit:  pkg.CacheHit,
			CacheMiss: pkg.CacheMiss,
		})
	}

	// Get cache statistics
	var fileCount, totalSizeBytes, packagesServed int64
	var lastUpdated time.Time
	if stats.GlobalStats != nil {
		fileCount, totalSizeBytes, packagesServed, lastUpdated = stats.GlobalStats.Get()
	}

	// Format last updated time
	lastUpdatedStr := "N/A"
	if !lastUpdated.IsZero() {
		lastUpdatedStr = lastUpdated.Format("Jan 02, 2006 15:04:05")
	}

	tmpl := template.Must(template.New("dashboard").Funcs(template.FuncMap{"add": add, "minus": minus}).Parse(dashboardHTML))
	tmpl.Execute(w, struct {
		DashboardData
		Filter string
	}{
		DashboardData: DashboardData{
			Title:          title,
			Packages:       dashPkgs,
			CurrentPage:    page,
			TotalPages:     (total + pageSize - 1) / pageSize,
			FileCount:      fileCount,
			CacheSize:      stats.FormatBytes(totalSizeBytes),
			PackagesServed: packagesServed,
			LastUpdated:    lastUpdatedStr,
		},
		Filter: filter,
	})
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/css/bootstrap.min.css" rel="stylesheet">
  <title>{{.Title}}</title>
  <style>
    .header-container {
      display: flex;
      justify-content: flex-start;
      align-items: center;
      gap: 6px;
      margin-bottom: 30px;
    }
    .header-container img {
      height: 96px;
      width: auto;
    }
    .stats-card {
      border: 1px solid #e0e0e0;
      border-radius: 8px;
      padding: 20px;
      background: #ffffff;
      box-shadow: 0 2px 4px rgba(0,0,0,0.04);
      transition: box-shadow 0.3s ease;
    }
    .stats-card:hover {
      box-shadow: 0 4px 12px rgba(0,0,0,0.08);
    }
    .stats-subtitle {
      font-size: 0.875rem;
      font-weight: 500;
      color: #6c757d;
      text-transform: uppercase;
      letter-spacing: 0.5px;
      margin-bottom: 8px;
    }
    .stats-value {
      font-size: 2rem;
      font-weight: 600;
      color: #212529;
      margin: 0;
    }
  </style>
</head>
<body>
<div class="container mt-5">
  <div class="header-container">
    <img src="/static/logo.svg" alt="PkgBin Logo">
    <h1 class="mb-0">{{.Title}}</h1>
  </div>
  
  <!-- Cache Statistics -->
  <div class="row mb-4">
    <div class="col-md-4 mb-3 mb-md-0">
      <div class="stats-card">
        <div class="stats-subtitle">Files in Cache</div>
        <h3 class="stats-value">{{.FileCount}}</h3>
      </div>
    </div>
    <div class="col-md-4 mb-3 mb-md-0">
      <div class="stats-card">
        <div class="stats-subtitle">Total Cache Size</div>
        <h3 class="stats-value">{{.CacheSize}}</h3>
      </div>
    </div>
    <div class="col-md-4">
      <div class="stats-card">
        <div class="stats-subtitle">Total Downloads</div>
        <h3 class="stats-value">{{.PackagesServed}}</h3>
      </div>
    </div>
  </div>
  <div class="row mb-3">
    <div class="col-12">
      <p class="text-muted small mb-0">Statistics updated: {{.LastUpdated}}</p>
    </div>
  </div>
  
  <form class="mb-3" method="get" action="/dashboard">
    <div class="input-group">
      <input type="text" class="form-control" name="filter" placeholder="Filter by package name" value="{{.Filter}}">
      <button class="btn btn-primary" type="submit">Filter</button>
    </div>
  </form>
  <div class="mb-3">
    <div class="dropdown">
      <button class="btn btn-secondary dropdown-toggle" type="button" id="actionsDropdown" data-bs-toggle="dropdown" aria-expanded="false">
        Actions
      </button>
      <ul class="dropdown-menu" aria-labelledby="actionsDropdown">
        <li><a class="dropdown-item" href="#" onclick="purgeAll(); return false;" data-bs-toggle="tooltip" data-bs-placement="right" title="Feel free to purge a package if you think it needs a refresh.">Purge all</a></li>
        <li><a class="dropdown-item" href="#" onclick="purgeSelected(); return false;" data-bs-toggle="tooltip" data-bs-placement="right" title="Feel free to purge a package if you think it needs a refresh.">Purge selected</a></li>
        <li><hr class="dropdown-divider"></li>
        <li><a class="dropdown-item" href="#" onclick="refreshDatabase(); return false;">Refresh Database</a></li>
        <li><a class="dropdown-item" href="#" onclick="showAbout(); return false;">About</a></li>
      </ul>
    </div>
  </div>
  <table class="table table-striped">
    <thead><tr><th><input type="checkbox" id="selectAll" onclick="toggleSelectAll()" data-bs-toggle="tooltip" data-bs-placement="top" title="Maximum 10 items can be selected"></th><th>Name</th><th>Cache Hit</th><th>Cache Miss</th></tr></thead>
    <tbody>
    {{range .Packages}}
      <tr>
        <td><input type="checkbox" class="package-checkbox" value="{{.Name}}" onclick="limitSelection()"></td>
        <td>{{.Name}}</td>
        <td>{{.CacheHit}}</td>
        <td>{{.CacheMiss}}</td>
      </tr>
    {{end}}
    </tbody>
  </table>
  <nav>
    <ul class="pagination">
      {{if gt .CurrentPage 1}}
        <li class="page-item"><a class="page-link" href="?page={{minus .CurrentPage 1}}&filter={{.Filter}}">Previous</a></li>
      {{end}}
      <li class="page-item active"><span class="page-link">Page {{.CurrentPage}} of {{.TotalPages}}</span></li>
      {{if lt .CurrentPage .TotalPages}}
        <li class="page-item"><a class="page-link" href="?page={{add .CurrentPage 1}}&filter={{.Filter}}">Next</a></li>
      {{end}}
    </ul>
  </nav>
</div>

<!-- About Modal -->
<div class="modal fade" id="aboutModal" tabindex="-1" aria-labelledby="aboutModalLabel" aria-hidden="true">
  <div class="modal-dialog modal-dialog-centered modal-lg">
    <div class="modal-content">
      <div class="modal-header bg-info text-white">
        <h5 class="modal-title" id="aboutModalLabel">About PkgBin</h5>
        <button type="button" class="btn-close btn-close-white" data-bs-dismiss="modal" aria-label="Close"></button>
      </div>
      <div class="modal-body">
        <p><strong>PkgBin</strong> is a package caching service.</p>
        
        <h6 class="mt-3"><strong>Configuration Instructions</strong></h6>
        <p>Please update your package manager to retrieve packages from this PkgBin installation:</p>
        
        <div class="mb-3">
          <strong>For Ruby Applications:</strong>
          <p class="mb-1">Modify your <code>Gemfile</code> to use:</p>
          <pre class="bg-light p-2 rounded"><code>source "{{"{{"}}pkgbin_for_rubygems_hostname{{"}}"}}"</code></pre>
        </div>
        
        <div class="mb-3">
          <strong>For NodeJS Applications (NPM):</strong>
          <p class="mb-1">Create a file named <code>.npmrc</code> at the root of your project with:</p>
          <pre class="bg-light p-2 rounded"><code>registry={{"{{"}}pkgbin_for_npm_hostname{{"}}"}}</code></pre>
        </div>
        
        <hr>
        <p><strong>Cache Purging Guidelines</strong></p>
        <p>You can purge individual packages using the "Purge selected" option. For full cache purging, please contact the site administrator.</p>
        <p class="text-muted mb-0"><small>Note: Purging the cache will delete cached files and remove database entries. Use with caution.</small></p>
        <p class="mb-0">Please feel free to share your feedback at <a href="mailto:pkgbin@proton.me">pkgbin@proton.me</a></p>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-primary" data-bs-dismiss="modal">Close</button>
      </div>
    </div>
  </div>
</div>

<!-- Purge All Modal -->
<div class="modal fade" id="purgeAllModal" tabindex="-1" aria-labelledby="purgeAllModalLabel" aria-hidden="true">
  <div class="modal-dialog modal-dialog-centered">
    <div class="modal-content">
      <div class="modal-header">
        <h5 class="modal-title" id="purgeAllModalLabel">Cache Purge Request</h5>
        <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
      </div>
      <div class="modal-body">
        <p>To proceed with purging the entire cache, please contact the site administrator.</p>
        <p class="text-muted mb-0"><small>Note: Individual package purging can be done using "Purge selected" option.</small></p>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Close</button>
      </div>
    </div>
  </div>
</div>

<!-- Selection Limit Modal -->
<div class="modal fade" id="selectionLimitModal" tabindex="-1" aria-labelledby="selectionLimitModalLabel" aria-hidden="true">
  <div class="modal-dialog modal-dialog-centered modal-sm">
    <div class="modal-content">
      <div class="modal-header">
        <h5 class="modal-title" id="selectionLimitModalLabel">Selection Limit Reached</h5>
        <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
      </div>
      <div class="modal-body">
        <p class="mb-0">You can select a maximum of 10 items at a time.</p>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-primary" data-bs-dismiss="modal">OK</button>
      </div>
    </div>
  </div>
</div>

<!-- Purge Confirmation Modal -->
<div class="modal fade" id="purgeConfirmModal" tabindex="-1" aria-labelledby="purgeConfirmModalLabel" aria-hidden="true">
  <div class="modal-dialog modal-dialog-centered">
    <div class="modal-content">
      <div class="modal-header bg-danger text-white">
        <h5 class="modal-title" id="purgeConfirmModalLabel">Confirm Package Purge</h5>
        <button type="button" class="btn-close btn-close-white" data-bs-dismiss="modal" aria-label="Close"></button>
      </div>
      <div class="modal-body">
        <p><strong>Are you sure you want to purge <span id="purgePackageCount"></span> selected package(s)?</strong></p>
        <p>This will:</p>
        <ul>
          <li>Delete packages from cache directory</li>
          <li>Remove packages from database</li>
        </ul>
        <p class="text-danger mb-0"><strong>This action cannot be undone.</strong></p>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancel</button>
        <button type="button" class="btn btn-danger" id="confirmPurgeBtn">Purge Packages</button>
      </div>
    </div>
  </div>
</div>

<!-- Purge Success Modal -->
<div class="modal fade" id="purgeSuccessModal" tabindex="-1" aria-labelledby="purgeSuccessModalLabel" aria-hidden="true">
  <div class="modal-dialog modal-dialog-centered">
    <div class="modal-content">
      <div class="modal-header bg-success text-white">
        <h5 class="modal-title" id="purgeSuccessModalLabel">Purge Successful</h5>
        <button type="button" class="btn-close btn-close-white" data-bs-dismiss="modal" aria-label="Close"></button>
      </div>
      <div class="modal-body">
        <p class="mb-0"><strong>Successfully purged <span id="purgedCount"></span> package(s).</strong></p>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-primary" data-bs-dismiss="modal" onclick="window.location.reload()">OK</button>
      </div>
    </div>
  </div>
</div>

<!-- Purge Error Modal -->
<div class="modal fade" id="purgeErrorModal" tabindex="-1" aria-labelledby="purgeErrorModalLabel" aria-hidden="true">
  <div class="modal-dialog modal-dialog-centered">
    <div class="modal-content">
      <div class="modal-header bg-danger text-white">
        <h5 class="modal-title" id="purgeErrorModalLabel">Purge Failed</h5>
        <button type="button" class="btn-close btn-close-white" data-bs-dismiss="modal" aria-label="Close"></button>
      </div>
      <div class="modal-body">
        <p class="mb-0"><span id="purgeErrorMessage"></span></p>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Close</button>
      </div>
    </div>
  </div>
</div>

<script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/js/bootstrap.bundle.min.js"></script>
<script>
  // Initialize Bootstrap tooltips
  document.addEventListener('DOMContentLoaded', function() {
    var tooltipTriggerList = [].slice.call(document.querySelectorAll('[data-bs-toggle="tooltip"]'));
    var tooltipList = tooltipTriggerList.map(function (tooltipTriggerEl) {
      return new bootstrap.Tooltip(tooltipTriggerEl);
    });
  });

  function toggleSelectAll() {
    const selectAll = document.getElementById('selectAll');
    const checkboxes = document.querySelectorAll('.package-checkbox');
    const isChecked = selectAll.checked;
    
    // Uncheck all first
    checkboxes.forEach(cb => cb.checked = false);
    
    // If selecting, check only top 10 items
    if (isChecked) {
      checkboxes.forEach((cb, index) => {
        if (index < 10) {
          cb.checked = true;
        }
      });
    }
  }

  function limitSelection() {
    const checkboxes = document.querySelectorAll('.package-checkbox');
    const checked = Array.from(checkboxes).filter(cb => cb.checked);
    if (checked.length > 10) {
      event.target.checked = false;
      const modal = new bootstrap.Modal(document.getElementById('selectionLimitModal'));
      modal.show();
    }
    updateSelectAllState();
  }

  function updateSelectAllState() {
    const selectAll = document.getElementById('selectAll');
    const checkboxes = document.querySelectorAll('.package-checkbox');
    const checked = Array.from(checkboxes).filter(cb => cb.checked);
    
    // Check if top 10 are all selected and others are not
    let isTop10Selected = true;
    checkboxes.forEach((cb, index) => {
      if (index < 10 && !cb.checked) isTop10Selected = false;
      if (index >= 10 && cb.checked) isTop10Selected = false;
    });
    
    selectAll.checked = isTop10Selected && checked.length === Math.min(10, checkboxes.length);
  }

  function purgeAll() {
    const modal = new bootstrap.Modal(document.getElementById('purgeAllModal'));
    modal.show();
  }

  function showAbout() {
    const modal = new bootstrap.Modal(document.getElementById('aboutModal'));
    modal.show();
  }

  function refreshDatabase() {
    if (!confirm('This will rebuild the entire database from cache files. This may take several minutes. Continue?')) {
      return;
    }
    
    // Send refresh request to backend
    fetch('/refresh-db', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      }
    })
    .then(response => response.json())
    .then(data => {
      if (data.success) {
        alert(data.message);
        // Wait a moment then reload to show updated data
        setTimeout(() => window.location.reload(), 2000);
      } else {
        alert('Refresh failed: ' + data.message);
      }
    })
    .catch(error => {
      alert('Failed to refresh database: ' + error.message);
    });
  }

  function purgeSelected() {
    const checkboxes = document.querySelectorAll('.package-checkbox:checked');
    if (checkboxes.length === 0) {
      return; // Do nothing if no checkboxes are checked
    }
    
    const packages = Array.from(checkboxes).map(cb => cb.value);
    
    // Update modal content
    document.getElementById('purgePackageCount').textContent = packages.length;
    
    // Show the confirmation modal
    const modal = new bootstrap.Modal(document.getElementById('purgeConfirmModal'));
    modal.show();
    
    // Set up the confirm button click handler
    document.getElementById('confirmPurgeBtn').onclick = function() {
      modal.hide();
      executePurge(packages);
    };
  }
  
  function executePurge(packages) {
    // Send purge request to backend
    fetch('/purge', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ packages: packages })
    })
    .then(response => response.json())
    .then(data => {
      if (data.success) {
        document.getElementById('purgedCount').textContent = (data.deleted ? data.deleted.length : 0);
        const successModal = new bootstrap.Modal(document.getElementById('purgeSuccessModal'));
        successModal.show();
      } else {
        document.getElementById('purgeErrorMessage').textContent = 'Error: ' + data.message;
        const errorModal = new bootstrap.Modal(document.getElementById('purgeErrorModal'));
        errorModal.show();
      }
    })
    .catch(error => {
      document.getElementById('purgeErrorMessage').textContent = 'Failed to purge packages: ' + error.message;
      const errorModal = new bootstrap.Modal(document.getElementById('purgeErrorModal'));
      errorModal.show();
    });
  }
</script>
</body>
</html>`

// Helper functions for template
func add(x, y int) int   { return x + y }
func minus(x, y int) int { return x - y }
