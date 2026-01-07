package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
)

const (
	registryRequestTimeout = 10 * time.Second
	cacheSuccessTTL        = 24 * time.Hour
	cacheFailureTTL        = 5 * time.Minute
	cacheNegativeTTL       = 6 * time.Hour
)

func newRegClient(hosts []config.Host) *regclient.RegClient {
	if len(hosts) == 0 {
		return regclient.New()
	}
	return regclient.New(regclient.WithConfigHost(hosts...))
}

func GetManifest(ctx context.Context, name string, hosts []config.Host) (manifest.Manifest, error) {
	rc := newRegClient(hosts)
	ref, err := ref.New(name)
	if err != nil {
		slog.Error("failed to parse image name", "image", name, "error", err)
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, registryRequestTimeout)
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

func DoesImageSupportArm64(ctx context.Context, cache Cache, name string, hosts []config.Host) bool {
	return DoesImageSupportPlatform(ctx, cache, name, "linux/arm64", hosts)
}

// DoesImageSupportPlatform checks if an image supports a specific platform
func DoesImageSupportPlatform(
	ctx context.Context,
	cache Cache,
	name string,
	platform string,
	hosts []config.Host,
) bool {
	cacheKey := name + ":" + platform
	if val, ok := cache.Get(cacheKey); ok {
		return val
	}

	m, err := GetManifest(ctx, name, hosts)
	if err != nil {
		slog.Error("failed to get manifest", "image", name, "error", err)
		cache.Set(cacheKey, false, cacheFailureTTL)
		return false
	}

	platforms, err := manifest.GetPlatformList(m)
	if err != nil {
		slog.Error("failed to get platforms for manifest", "image", name, "error", err)
		cache.Set(cacheKey, false, cacheFailureTTL)
		return false
	}

	for _, pl := range platforms {
		if pl.String() == platform {
			cache.Set(cacheKey, true, cacheSuccessTTL)
			return true
		}
	}
	cache.Set(cacheKey, false, cacheNegativeTTL)
	return false
}
