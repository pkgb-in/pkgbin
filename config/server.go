package config

type ServerConfig struct {
	Host string `json:"host"`
	Port string `json:"port"`
}

var Server = ServerConfig{
	Host: "0.0.0.0",
	Port: "8080",
}
