# k8smultiarcher

k8smultiarcher is a small utility for working with multi-architecture Kubernetes images/manifests. It is a Kubernetes mutating admission webhook that automatically adds tolerations to Pods and DaemonSets whose container images support specific architectures (e.g., ARM64, AMD64). This enables workloads to be scheduled on nodes with taints for specific architectures. This repository includes Kubernetes manifests under `manifests/` and provides a Dockerfile and GitHub Actions workflow that builds multi-architecture images and publishes them to GitHub Container Registry (ghcr.io).

## Configuration

| Environment Variable | Description |
| -------------------- | ----------- |
| CACHE_SIZE           | Sets the size of the cache. If not provided, a default size is used. |
| CACHE                | Determines the type of cache to use. Can be either 'inmemory' or 'redis'. If not provided or set to 'inmemory', an in-memory cache is used. |
| REDIS_ADDR           | Sets the address of the Redis server. Used when CACHE is set to 'redis'. If not provided, a default address is used. |
| HOST                 | Sets the host for the server. |
| PORT                 | Sets the port for the server. If not provided, the default is '8443' if TLS is enabled, '8080' otherwise. |
| TLS_ENABLED          | Determines whether TLS is enabled. If set to 'true', TLS is enabled. |
| CERT_PATH            | Sets the path to the TLS certificate. Used when TLS_ENABLED is set to 'true'. If not provided, the default is './certs/tls.crt'. |
| KEY_PATH             | Sets the path to the TLS key. Used when TLS_ENABLED is set to 'true'. If not provided, the default is './certs/tls.key'. |
| PLATFORM_TOLERATIONS | JSON array defining platform-to-toleration mappings. See [Platform Tolerations Configuration](#platform-tolerations-configuration). |
| TOLERATION_KEY       | (Simple config) The key for a single toleration. If set, overrides the default toleration. |
| TOLERATION_VALUE     | (Simple config) The value for a single toleration. Used with TOLERATION_KEY. |
| TOLERATION_OPERATOR  | (Simple config) The operator for a single toleration (default: "Equal"). Used with TOLERATION_KEY. |
| TOLERATION_EFFECT    | (Simple config) The effect for a single toleration (default: "NoSchedule"). Used with TOLERATION_KEY. |
| TOLERATION_PLATFORM  | (Simple config) The platform for a single toleration (default: "linux/arm64"). Used with TOLERATION_KEY. |

### Platform Tolerations Configuration

k8smultiarcher can be configured to handle multiple platform architectures with custom tolerations. By default, it adds a toleration for `linux/arm64` with key `k8smultiarcher` and value `arm64Supported`.

#### Simple Configuration (Single Platform)

For a single custom platform-to-toleration mapping, use the simple environment variables:

```bash
TOLERATION_KEY=kubernetes.io/arch
TOLERATION_VALUE=arm64
TOLERATION_PLATFORM=linux/arm64
```

#### Advanced Configuration (Multiple Platforms)

For multiple platforms, use the `PLATFORM_TOLERATIONS` JSON configuration:

```bash
PLATFORM_TOLERATIONS='[
  {
    "platform": "linux/arm64",
    "key": "kubernetes.io/arch",
    "value": "arm64",
    "operator": "Equal",
    "effect": "NoSchedule"
  },
  {
    "platform": "linux/amd64",
    "key": "kubernetes.io/arch",
    "value": "amd64",
    "operator": "Equal",
    "effect": "NoSchedule"
  },
  {
    "platform": "linux/arm/v7",
    "key": "kubernetes.io/arch",
    "value": "arm",
    "operator": "Equal",
    "effect": "NoSchedule"
  }
]'
```

Each mapping in the JSON array supports:
- `platform` (required): The OCI platform string (e.g., "linux/arm64", "linux/amd64")
- `key` (required): The toleration key
- `value` (optional): The toleration value
- `operator` (optional): The toleration operator (default: "Equal")
- `effect` (optional): The toleration effect (default: "NoSchedule")

#### How It Works

1. When a Pod or DaemonSet is created, k8smultiarcher inspects all container images
2. For each configured platform, it checks if all images support that platform
3. If all images support a platform, the corresponding toleration is added
4. Multiple tolerations can be added if the images support multiple configured platforms

## Container Images

Multi-architecture container images are available at `ghcr.io/programmerq/k8smultiarcher` supporting linux/amd64, linux/arm64, and linux/arm/v7 platforms.

Image tags:
- `latest` - most recent version tag release
- `main` - most recent build from the main branch
- `<commit-sha>` - specific commit builds
- `v*` - version tags (e.g., v1.0.0)

## Deployment

Reference Kubernetes manifests are available in the `manifests/` directory and use the GHCR image `ghcr.io/programmerq/k8smultiarcher:latest`.
