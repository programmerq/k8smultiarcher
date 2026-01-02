package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
)

func GetManifest(name string) (manifest.Manifest, error) {
	rc := regclient.New()
	ref, err := ref.New(name)
	if err != nil {
		slog.Error("failed to parse image name", "image", name, "error", err)
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	m, err := rc.ManifestGet(ctx, ref)
	if err != nil {
		slog.Error("failed to get manifest", "image", name, "error", err)
		return nil, err
	}

	if !m.IsList() {
		err := fmt.Errorf("provided image name has no manifest list")
		slog.Error("image has no manifest list", "image", name, "error", err)
		return nil, err
	}
	return m, nil
}

func DoesImageSupportArm64(cache Cache, name string) bool {
	return DoesImageSupportPlatform(cache, name, "linux/arm64")
}

// DoesImageSupportPlatform checks if an image supports a specific platform
func DoesImageSupportPlatform(cache Cache, name string, platform string) bool {
	cacheKey := name + ":" + platform
	if val, ok := cache.Get(cacheKey); ok {
		return val
	}

	m, err := GetManifest(name)
	if err != nil {
		slog.Error("failed to get manifest", "image", name, "error", err)
		cache.Set(cacheKey, false)
		return false
	}

	platforms, err := manifest.GetPlatformList(m)
	if err != nil {
		slog.Error("failed to get platforms for manifest", "image", name, "error", err)
		cache.Set(cacheKey, false)
		return false
	}

	for _, pl := range platforms {
		if pl.String() == platform {
			cache.Set(cacheKey, true)
			return true
		}
	}
	cache.Set(cacheKey, false)
	return false
}
