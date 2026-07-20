# MitraChat — Frontend

Vue 3 + TypeScript + Vite + Tailwind v4 client for MitraChat. Proxies `/api` and `/ws`
to the Go backend on `:3000`.

## Structure
- `src/api` — axios client (JWT interceptor)
- `src/ws` — WebSocket socket (auto-reconnect + event bus)
- `src/stores` — Pinia stores, one per domain
- `src/views`, `src/components`, `src/layouts` — UI
- `src/router` — routes
- `src/types.ts` — shared types

## Setup
```sh
npm install
```

### Dev (hot-reload)
```sh
npm run dev        # http://localhost:5173
```

### Type-check + build
```sh
npm run type-check
npm run build
```

### Unit tests (Vitest)
```sh
npm run test:unit
```

### E2E (Playwright)
```sh
npx playwright install   # first run
npm run build            # required on CI
npm run test:e2e
npm run test:e2e -- --project=chromium
```

### Lint
```sh
npm run lint
```

## IDE
[VS Code](https://code.visualstudio.com/) + [Vue (Official)](https://marketplace.visualstudio.com/items?itemName=Vue.volar) (disable Vetur).
`.vue` type-checking uses `vue-tsc` in place of `tsc`.
