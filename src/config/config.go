package config

import (
	"log"
	"os"
)

type Config struct {
	DSN        string
	SecretKey  string
	Port       string
	DockerHost string
}

func Load() *Config {
	return &Config{
		DSN:        buildDSN(),
		SecretKey:  requireEnv("SECRET_KEY"),
		Port:       requireEnv("SERVER_PORT"),
		DockerHost: getEnv("DOCKER_HOST", "unix:///var/run/docker.sock"),
	}
}

func buildDSN() string {
	host := requireEnv("DB_HOST")
	port := getEnv("DB_PORT", "3306")
	user := requireEnv("DB_USER")
	pass := requireEnv("DB_PASSWORD")
	name := requireEnv("DB_NAME")
	return user + ":" + pass + "@tcp(" + host + ":" + port + ")/" + name + "?parseTime=true&charset=utf8mb4&loc=Local"
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("Variable d'environnement requise manquante : %s", key)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
