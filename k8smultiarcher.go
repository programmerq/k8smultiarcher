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

var (
	cache              Cache
	platformConfig     *PlatformTolerationConfig
	namespaceFilterCfg *NamespaceFilterConfig
)

func main() {
	configureCache()

	var err error
	platformConfig, err = LoadPlatformTolerationConfig()
	if err != nil {
		slog.Error("failed to load platform toleration config", "error", err)
		os.Exit(1)
	}
	namespaceFilterCfg, err = LoadNamespaceFilterConfig()
	if err != nil {
		slog.Error("failed to load namespace filter config", "error", err)
		os.Exit(1)
	}

	startServer(newRouter())
}

// newRouter builds the gin engine with all webhook routes registered.
func newRouter() *gin.Engine {
	r := gin.Default()
	r.POST("/mutate", mutateHandler)
	r.GET("/healthz", healthzHandler)
	r.GET("/livez", livezHandler)
	return r
}

func mutateHandler(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		slog.Error("failed to read request body", "error", err)
		c.JSON(400, gin.H{"error": "invalid request body"})
		return
	}

	review, err := ProcessAdmissionReview(c.Request.Context(), cache, platformConfig, namespaceFilterCfg, body)
	if err != nil {
		slog.Error("failed to process admission review", "error", err)
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
	c, err := newCacheFromEnv()
	if err != nil {
		slog.Error("failed to configure cache", "error", err)
		os.Exit(1)
	}
	cache = c
}

// newCacheFromEnv builds the cache backend from the CACHE, CACHE_SIZE, and
// REDIS_ADDR environment variables. It returns an error instead of exiting so
// the selection and parsing logic can be unit-tested.
func newCacheFromEnv() (Cache, error) {
	cacheSizeStr := cmp.Or(os.Getenv("CACHE_SIZE"), strconv.Itoa(cacheSizeDefault))
	cacheSize, err := strconv.Atoi(cacheSizeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid cache size %q: %w", cacheSizeStr, err)
	}

	cacheChoice := cmp.Or(os.Getenv("CACHE"), "inmemory")
	switch cacheChoice {
	case "inmemory":
		slog.Info("using in-memory cache", "size", cacheSize)
		return NewInMemoryCache(cacheSize), nil
	case "redis":
		redisAddr := cmp.Or(os.Getenv("REDIS_ADDR"), redisAddrDefault)
		slog.Info("using redis cache", "addr", redisAddr)
		return NewRedisCache(redisAddr), nil
	default:
		return nil, fmt.Errorf("invalid cache choice %q", cacheChoice)
	}
}

// serverSettings holds the resolved listen address and TLS configuration.
type serverSettings struct {
	addr       string
	certPath   string
	keyPath    string
	tlsEnabled bool
}

// serverSettingsFromEnv derives the listen address and TLS settings from HOST,
// PORT, TLS_ENABLED, CERT_PATH, and KEY_PATH, applying the startup defaults.
// Extracted from startServer so the resolution logic is unit-testable.
func serverSettingsFromEnv() serverSettings {
	host := os.Getenv("HOST")
	port := os.Getenv("PORT")
	tlsEnabled := os.Getenv("TLS_ENABLED") == "true"
	if port == "" {
		if tlsEnabled {
			port = "8443"
		} else {
			port = "8080"
		}
	}
	s := serverSettings{
		addr:       fmt.Sprintf("%s:%s", host, port),
		tlsEnabled: tlsEnabled,
	}
	if tlsEnabled {
		s.certPath = cmp.Or(os.Getenv("CERT_PATH"), "./certs/tls.crt")
		s.keyPath = cmp.Or(os.Getenv("KEY_PATH"), "./certs/tls.key")
	}
	return s
}

func startServer(r *gin.Engine) {
	s := serverSettingsFromEnv()
	if s.tlsEnabled {
		if err := r.RunTLS(s.addr, s.certPath, s.keyPath); err != nil {
			slog.Error("failed to start TLS server", "error", err)
			os.Exit(1)
		}
		return
	}
	if err := r.Run(s.addr); err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}
}
