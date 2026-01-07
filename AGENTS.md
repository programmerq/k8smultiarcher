# Agent Guidelines

- Format Go code with `gofmt` (or `gofumpt`) before committing.
- Keep line lengths at or below 120 characters; wrap long function signatures and log calls.
- Avoid repeated magic strings in tests; prefer `const` declarations for shared values.
- Run `golangci-lint` (or the CI lint target) locally when changing Go code.
