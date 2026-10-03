# README source audit — 2026-10-03

- Zenith pin before documentation edits: `7d1d25b72cdd1d3e5e26e80200236d0e16252aa3`.
- `git ls-remote origin HEAD` returned that same SHA. No newer default-branch source was observed; no Zenith gitlink was changed.
- Release evidence: https://github.com/bigduu/bodhi-server/releases . Public release page explicitly says “There aren’t any releases here”; no release version is claimed. Manifest placeholder versions are not published versions.
- GitHub API requests were blocked by this environment's proxy (403); the public release HTML was retrieved successfully instead.
- Source evidence: `docker-compose.yml`, `Dockerfile`, `go.mod`, `internal/config/config.go`, `api/router/router.go`, proxy router/handler, and `cmd/server/web/{package.json,vite.config.ts,src/services/api.ts}`.
- Scope: README files and this audit only. No product code, releases, credentials, remote branches, or deployment settings changed.
- Validation: local Markdown targets and `git diff --check`; command names and requirements compared with checked-in source. No real IM credentials, PostgreSQL service, provider calls, or native macOS/Windows behavior were exercised by this review.
