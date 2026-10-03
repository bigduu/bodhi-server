# Bodhi Server administration UI

This React + TypeScript interface lets users manage API keys, provider credentials,
and billing, and gives administrators access to user, model, quota, and service
settings. It belongs to the optional [Bodhi Server](../../../README.md), separate
from the Lotus Next agent interface.

## Build and run with the server

From this directory, with Node.js 22.12+ and npm installed:

```bash
npm ci
npm run build
cd ../../..
go run ./cmd/server
```

Set up PostgreSQL and the required environment variables using the
[server quickstart](../../../README.md#quick-start--development) first. The Go
server embeds `dist/` at compile time and serves the UI at `http://localhost:8080`.
Rebuild the frontend and restart the Go process after changes.

## Frontend development

```bash
npm run dev
npm run lint
npm run preview  # inspect an existing production build
```

API requests use relative `/api/v1` URLs. The current Vite configuration has no
backend proxy, so the dev or preview server alone is not a working account or
billing service. Use the embedded build for same-origin integration. Provider
requests require your own configured server; no demo credentials are included.
