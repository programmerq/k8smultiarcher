# k8smultiarcher

k8smultiarcher is a small utility for working with multi-architecture Kubernetes images/manifests. This repository includes Kubernetes manifests under `manifests/` and now provides a minimal Dockerfile and a GitHub Actions workflow that builds multi-architecture images and publishes them to GitHub Container Registry (ghcr.io).

Considerations for deploying:
- The included manifests reference images hosted under a `kind.local` registry. Before deploying to a real cluster replace image references in `manifests/` with the GHCR image (for example: `ghcr.io/programmerq/k8smultiarcher:latest`) or load the image into your local Kind cluster (e.g., `kind load docker-image ghcr.io/programmerq/k8smultiarcher:latest`).
- The Dockerfile assumes a Go project and will build a static binary. If this repository is not Go, modify the Dockerfile to use your project's build steps.
- The workflow uses the repository's GITHUB_TOKEN and sets `packages: write` permission so the action can push to GHCR. You can also create a personal access token with package write permission if you need cross-repository pushes.

How the workflow works (quick):
- On push to `main` or on tag `v*`, the workflow builds for linux/amd64, linux/arm64, and linux/arm/v7 and pushes multi-arch images to ghcr.io under your account.
- Image tags pushed: `latest` and the commit SHA.

If you'd like me to:
- Adjust the Dockerfile for another language/tooling (Node, Python, Java, etc.), tell me which one and I will update it.
- Replace the images in `manifests/` to use the GHCR name automatically, I can create a small patch that updates those files.
