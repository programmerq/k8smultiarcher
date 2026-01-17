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
| NAMESPACE_SELECTOR   | Label selector to filter namespaces to watch (e.g., `environment=prod` or `team in (platform,infra)`). See [Namespace Filtering](#namespace-filtering). |
| NAMESPACES_TO_IGNORE | Comma-separated list of namespace names to skip from mutation (e.g., `kube-system,kube-public`). See [Namespace Filtering](#namespace-filtering). |

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

## Opt-Out and Per-Namespace Control

k8smultiarcher supports opt-out mechanisms at both the workload and namespace levels to prevent mutation when needed.

### Pod-Level Opt-Out

You can prevent k8smultiarcher from mutating a specific Pod or DaemonSet by adding the `k8smultiarcher.programmerq.io/skip-mutation` annotation with value `"true"`:

**Pod Example:**

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: mypod
  annotations:
    k8smultiarcher.programmerq.io/skip-mutation: "true"
spec:
  containers:
    - name: busybox
      image: busybox
```

**DaemonSet Example:**

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: mydaemonset
spec:
  template:
    metadata:
      annotations:
        k8smultiarcher.programmerq.io/skip-mutation: "true"
    spec:
      containers:
        - name: busybox
          image: busybox
```

### Namespace-Level Disable

You can disable mutation for all Pods and DaemonSets in a namespace by annotating the namespace with `k8smultiarcher.programmerq.io/disabled` set to `"true"`:

```bash
kubectl annotate namespace my-namespace k8smultiarcher.programmerq.io/disabled="true"
```

Or in a Namespace manifest:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: my-namespace
  annotations:
    k8smultiarcher.programmerq.io/disabled: "true"
```

**Note:** The namespace check requires the webhook to have `get` permission on `namespaces` resources (included in the example manifests). If the namespace lookup fails, the webhook will default to not skipping mutation and log the error.

## Namespace Filtering

k8smultiarcher supports advanced namespace filtering similar to stakater/Reloader, allowing you to control which namespaces the webhook processes using label selectors or an ignore list.

### Configuration

Namespace filtering is configured via environment variables:

**`NAMESPACE_SELECTOR`** - Watch only namespaces matching this label selector

Examples:
```bash
# Single label match
NAMESPACE_SELECTOR='environment=production'

# Multiple labels (all must match)
NAMESPACE_SELECTOR='environment=production,team=platform'

# Set-based selector
NAMESPACE_SELECTOR='environment in (production,staging)'
```

**`NAMESPACES_TO_IGNORE`** - Skip specific namespaces (comma-separated list)

Examples:
```bash
# Ignore system namespaces
NAMESPACES_TO_IGNORE='kube-system,kube-public'

# Ignore multiple custom namespaces
NAMESPACES_TO_IGNORE='monitoring,logging,default'
```

### How It Works

The webhook evaluates namespace filters in the following order:

1. **Ignore list check**: If a namespace is in `NAMESPACES_TO_IGNORE`, mutation is skipped (no API call needed)
2. **Selector check**: If `NAMESPACE_SELECTOR` is configured, the namespace must match the selector for mutation to proceed
3. **Annotation check**: The namespace annotation `k8smultiarcher.programmerq.io/disabled` is checked (if enabled via annotation, mutation is skipped)
4. **Pod annotation check**: The pod-level `k8smultiarcher.programmerq.io/skip-mutation` annotation is checked

All filters can be used together. A namespace must pass all configured filters for mutation to occur.

### Examples

**Watch only production namespaces:**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: k8smultiarcher
spec:
  template:
    spec:
      containers:
        - name: k8smultiarcher
          image: ghcr.io/programmerq/k8smultiarcher:latest
          env:
            - name: NAMESPACE_SELECTOR
              value: "environment=production"
```

**Ignore system namespaces:**

```yaml
env:
  - name: NAMESPACES_TO_IGNORE
    value: "kube-system,kube-public,kube-node-lease"
```

**Combine selector and ignore list:**

```yaml
env:
  - name: NAMESPACE_SELECTOR
    value: "managed-by=k8smultiarcher"
  - name: NAMESPACES_TO_IGNORE
    value: "dev-sandbox,test-temp"
```

This configuration watches only namespaces with label `managed-by=k8smultiarcher` while explicitly ignoring `dev-sandbox` and `test-temp`.

## Container Images

Multi-architecture container images are available at `ghcr.io/programmerq/k8smultiarcher` supporting linux/amd64, linux/arm64, and linux/arm/v7 platforms.

Image tags:
- `latest` - most recent version tag release
- `main` - most recent build from the main branch
- `<commit-sha>` - specific commit builds
- `v*` - version tags (e.g., v1.0.0)

## Deployment

Reference Kubernetes manifests are available in the `manifests/` directory and use the GHCR image `ghcr.io/programmerq/k8smultiarcher:latest`.
