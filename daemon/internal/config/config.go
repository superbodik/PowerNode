package config

import "os"

type Config struct {
	NodeUUID     string
	DaemonToken  string
	HTTPAddr     string
	TLSCertFile  string
	TLSKeyFile   string
	DockerSocket string
	DataDir      string
}

func Load() Config {
	return Config{
		NodeUUID:     os.Getenv("WINGSD_NODE_UUID"),
		DaemonToken:  os.Getenv("WINGSD_DAEMON_TOKEN"),
		HTTPAddr:     getEnv("WINGSD_HTTP_ADDR", ":8443"),
		TLSCertFile:  os.Getenv("WINGSD_TLS_CERT"),
		TLSKeyFile:   os.Getenv("WINGSD_TLS_KEY"),
		DockerSocket: getEnv("WINGSD_DOCKER_SOCKET", ""),
		DataDir:      getEnv("WINGSD_DATA_DIR", "/var/lib/wingsd/servers"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
