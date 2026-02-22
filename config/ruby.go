package config

type RubyGemsProxyConfig struct {
	Upstream string `json:"upstream"`
	CacheDir string `json:"cache_dir"`
}

var RubyGemsConfig = RubyGemsProxyConfig{
	Upstream: "https://rubygems.org",
	CacheDir: "./gem_cache_data",
}
