package config

type NPMProxyConfig struct {
	Upstream string `json:"upstream"`
	CacheDir string `json:"cache_dir"`
}

var NPMConfig = NPMProxyConfig{
	Upstream: "https://registry.npmjs.org",
	CacheDir: "./npm_cache_data",
}
