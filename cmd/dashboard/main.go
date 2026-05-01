package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/extremenetworks/testcase-generator/pkg/web"
)

func main() {
	cfg := web.DefaultConfig()

	flag.IntVar(&cfg.Port, "port", envInt("DASHBOARD_PORT", 8080), "Server port")
	flag.StringVar(&cfg.MinIOEndpoint, "minio-endpoint", envStr("MINIO_ENDPOINT", "localhost:9000"), "MinIO endpoint")
	flag.StringVar(&cfg.MinIOAccessKey, "minio-access-key", envStr("MINIO_ACCESS_KEY", "minioadmin"), "MinIO access key")
	flag.StringVar(&cfg.MinIOSecretKey, "minio-secret-key", envStr("MINIO_SECRET_KEY", "minioadmin123"), "MinIO secret key")
	flag.StringVar(&cfg.MinIOBucket, "minio-bucket", envStr("MINIO_BUCKET", "testplans"), "MinIO bucket name")
	flag.BoolVar(&cfg.MinIOUseSSL, "minio-ssl", false, "Use SSL for MinIO")
	flag.StringVar(&cfg.TestPlansDir, "testplans-dir", envStr("TESTPLANS_DIR", "./Testplans"), "Path to test plans directory")
	flag.StringVar(&cfg.ReportsDir, "reports-dir", envStr("REPORTS_DIR", "./reports"), "Path to reports directory")
	flag.StringVar(&cfg.SourcesDir, "sources-dir", envStr("SOURCES_DIR", "./sources"), "Path to source files directory")
	flag.Parse()

	server, err := web.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Fatal(server.Run())
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var i int
	_, err := fmt.Sscanf(v, "%d", &i)
	if err != nil {
		return fallback
	}
	return i
}
