package config

type PyPIProxyConfig struct {
	Upstream string `json:"upstream"`
	CacheDir string `json:"cache_dir"`
}

var PyPIConfig = PyPIProxyConfig{
	Upstream: "https://pypi.org",
	CacheDir: "./pypi_cache_data",
}
