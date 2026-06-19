package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	admissionv1 "k8s.io/api/admission/v1"
)

// newTestRouter returns the production router wired for tests, with gin in test
// mode and its loggers silenced.
func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard
	return newRouter()
}

// errReader is an io.Reader whose Read always fails, used to exercise the
// body-read error path of mutateHandler.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("simulated read failure") }

func TestNewCacheFromEnv(t *testing.T) {
	t.Run("defaults to in-memory", func(t *testing.T) {
		t.Setenv("CACHE", "")
		t.Setenv("CACHE_SIZE", "")
		c, err := newCacheFromEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := c.(*InMemoryCache); !ok {
			t.Fatalf("expected *InMemoryCache, got %T", c)
		}
	})

	t.Run("in-memory with custom size", func(t *testing.T) {
		t.Setenv("CACHE", "inmemory")
		t.Setenv("CACHE_SIZE", "5")
		c, err := newCacheFromEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := c.(*InMemoryCache); !ok {
			t.Fatalf("expected *InMemoryCache, got %T", c)
		}
	})

	t.Run("redis", func(t *testing.T) {
		t.Setenv("CACHE", "redis")
		t.Setenv("REDIS_ADDR", "localhost:6390")
		c, err := newCacheFromEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := c.(*RedisCache); !ok {
			t.Fatalf("expected *RedisCache, got %T", c)
		}
	})

	t.Run("invalid cache choice", func(t *testing.T) {
		t.Setenv("CACHE", "bogus")
		if _, err := newCacheFromEnv(); err == nil {
			t.Fatal("expected an error for an invalid CACHE value")
		}
	})

	t.Run("invalid cache size", func(t *testing.T) {
		t.Setenv("CACHE_SIZE", "not-a-number")
		if _, err := newCacheFromEnv(); err == nil {
			t.Fatal("expected an error for an invalid CACHE_SIZE value")
		}
	})
}

func TestServerSettingsFromEnv(t *testing.T) {
	t.Run("non-tls defaults", func(t *testing.T) {
		t.Setenv("HOST", "")
		t.Setenv("PORT", "")
		t.Setenv("TLS_ENABLED", "")
		s := serverSettingsFromEnv()
		if s.tlsEnabled {
			t.Errorf("tlsEnabled = true, want false")
		}
		if s.addr != ":8080" {
			t.Errorf("addr = %q, want :8080", s.addr)
		}
		if s.certPath != "" || s.keyPath != "" {
			t.Errorf("expected empty cert/key paths, got cert=%q key=%q", s.certPath, s.keyPath)
		}
	})

	t.Run("tls defaults port and cert paths", func(t *testing.T) {
		t.Setenv("HOST", "")
		t.Setenv("PORT", "")
		t.Setenv("TLS_ENABLED", "true")
		s := serverSettingsFromEnv()
		if !s.tlsEnabled {
			t.Errorf("tlsEnabled = false, want true")
		}
		if s.addr != ":8443" {
			t.Errorf("addr = %q, want :8443", s.addr)
		}
		if s.certPath != "./certs/tls.crt" || s.keyPath != "./certs/tls.key" {
			t.Errorf("got cert=%q key=%q, want defaults", s.certPath, s.keyPath)
		}
	})

	t.Run("host and port composed", func(t *testing.T) {
		t.Setenv("HOST", "127.0.0.1")
		t.Setenv("PORT", "9999")
		t.Setenv("TLS_ENABLED", "")
		s := serverSettingsFromEnv()
		if s.addr != "127.0.0.1:9999" {
			t.Errorf("addr = %q, want 127.0.0.1:9999", s.addr)
		}
	})

	t.Run("tls custom cert and key paths", func(t *testing.T) {
		t.Setenv("TLS_ENABLED", "true")
		t.Setenv("CERT_PATH", "/etc/tls/my.crt")
		t.Setenv("KEY_PATH", "/etc/tls/my.key")
		s := serverSettingsFromEnv()
		if s.certPath != "/etc/tls/my.crt" || s.keyPath != "/etc/tls/my.key" {
			t.Errorf("got cert=%q key=%q, want custom paths", s.certPath, s.keyPath)
		}
	})
}

func TestHealthzAndLivezHandlers(t *testing.T) {
	router := newTestRouter(t)
	for _, path := range []string{"/healthz", "/livez"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", path, w.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: decode body: %v", path, err)
		}
		if body["status"] != "ok" {
			t.Errorf("%s: status = %q, want ok", path, body["status"])
		}
	}
}

func TestMutateHandler_Success(t *testing.T) {
	c := NewInMemoryCache(cacheSizeDefault)
	c.Set(goldenImage+":linux/arm64", true, 0)
	c.Set(goldenImage+":linux/amd64", true, 0)
	cache = c
	platformConfig = goldenConfig()
	namespaceFilterCfg = nil

	router := newTestRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(goldenPodBody(t)))
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(w.Body.Bytes(), &review); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if review.Response == nil || !review.Response.Allowed {
		t.Fatalf("expected an allowed response, got %+v", review.Response)
	}
	if len(review.Response.Patch) == 0 {
		t.Error("expected a non-empty patch for a multi-arch image")
	}
}

func TestMutateHandler_ProcessError(t *testing.T) {
	cache = NewInMemoryCache(cacheSizeDefault)
	platformConfig = goldenConfig()
	namespaceFilterCfg = nil

	router := newTestRouter(t)
	w := httptest.NewRecorder()
	// An AdmissionReview with no Request fails AdmissionReviewFromRequest.
	req := httptest.NewRequest(http.MethodPost, "/mutate", strings.NewReader(`{}`))
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
}

func TestMutateHandler_BodyReadError(t *testing.T) {
	cache = NewInMemoryCache(cacheSizeDefault)
	platformConfig = goldenConfig()
	namespaceFilterCfg = nil

	router := newTestRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mutate", errReader{})
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}
