package main

import "testing"

func TestDoesImageSupportArm64(t *testing.T) {
	cache := NewInMemoryCache(cacheSizeDefault)
	cache.Set("image_with_arm_support:linux/arm64", true)
	cache.Set("image_without_arm_support:linux/arm64", false)

	type args struct {
		cache Cache
		name  string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "image supports arm64",
			args: args{
				cache: cache,
				name:  "image_with_arm_support",
			},
			want: true,
		},
		{
			name: "image that does not support arm64",
			args: args{
				cache: cache,
				name:  "image_without_arm_support",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DoesImageSupportArm64(tt.args.cache, tt.args.name); got != tt.want {
				t.Errorf("DoesImageSupportArm64() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDoesImageSupportPlatform(t *testing.T) {
	cache := NewInMemoryCache(cacheSizeDefault)
	cache.Set("multi_arch_image:linux/arm64", true)
	cache.Set("multi_arch_image:linux/amd64", true)
	cache.Set("arm_only_image:linux/arm64", true)
	cache.Set("arm_only_image:linux/amd64", false)

	type args struct {
		cache    Cache
		name     string
		platform string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "multi-arch image supports arm64",
			args: args{
				cache:    cache,
				name:     "multi_arch_image",
				platform: "linux/arm64",
			},
			want: true,
		},
		{
			name: "multi-arch image supports amd64",
			args: args{
				cache:    cache,
				name:     "multi_arch_image",
				platform: "linux/amd64",
			},
			want: true,
		},
		{
			name: "arm-only image supports arm64",
			args: args{
				cache:    cache,
				name:     "arm_only_image",
				platform: "linux/arm64",
			},
			want: true,
		},
		{
			name: "arm-only image does not support amd64",
			args: args{
				cache:    cache,
				name:     "arm_only_image",
				platform: "linux/amd64",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DoesImageSupportPlatform(tt.args.cache, tt.args.name, tt.args.platform); got != tt.want {
				t.Errorf("DoesImageSupportPlatform() = %v, want %v", got, tt.want)
			}
		})
	}
}
