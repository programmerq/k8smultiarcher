package main

import (
	"cmp"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
)

var cache Cache
var platformConfig *PlatformTolerationConfig

func main() {
	configureCache()
	platformConfig = LoadPlatformTolerationConfig()

	r := gin.Default()
	r.POST("/mutate", mutateHandler)
	r.GET("/healthz", healthzHandler)
	r.GET("/livez", livezHandler)
	startServer(r)
}

func mutateHandler(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		slog.Error("failed to read request body", "error", err)
		c.JSON(400, gin.H{"error": "invalid request body"})
		return
	}

	review, err := ProcessAdmissionReview(cache, platformConfig, body)
	if err != nil {
		slog.Error("failed to process pod admission review", "error", err)
		c.JSON(500, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(200, review)
}

func healthzHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": "ok",
	})
}

func livezHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": "ok",
	})
}

func configureCache() {
	cacheSizeStr := cmp.Or(os.Getenv("CACHE_SIZE"), strconv.Itoa(cacheSizeDefault))
	cacheSize, err := strconv.Atoi(cacheSizeStr)
	if err != nil {
		slog.Error("invalid cache size", "value", cacheSizeStr, "error", err)
		os.Exit(1)
	}

	cacheChoice := cmp.Or(os.Getenv("CACHE"), "inmemory")
	switch cacheChoice {
	case "inmemory":
		slog.Info("using in-memory cache", "size", cacheSize)
		cache = NewInMemoryCache(cacheSize)
	case "redis":
		redisAddr := cmp.Or(os.Getenv("REDIS_ADDR"), redisAddrDefault)
		slog.Info("using redis cache", "addr", redisAddr)
		cache = NewRedisCache(redisAddr)
	default:
		slog.Error("invalid cache choice", "value", cacheChoice)
		os.Exit(1)
	}
}

func startServer(r *gin.Engine) {
	host := os.Getenv("HOST")
	port := os.Getenv("PORT")
	tlsEnabled := os.Getenv("TLS_ENABLED")
	if port == "" {
		if tlsEnabled == "true" {
			port = "8443"
		} else {
			port = "8080"
		}
	}
	addr := fmt.Sprintf("%s:%s", host, port)
	if tlsEnabled == "true" {
		var certPath, keyPath string
		if certPath = os.Getenv("CERT_PATH"); certPath == "" {
			certPath = "./certs/tls.crt"
		}
		if keyPath = os.Getenv("KEY_PATH"); keyPath == "" {
			keyPath = "./certs/tls.key"
		}
		r.RunTLS(addr, certPath, keyPath)
	}
	r.Run(addr)
}
