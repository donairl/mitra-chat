# MitraChat

A Discord-like real-time chat app. **Vue 3 + TypeScript + Tailwind v4** frontend,
**Go (Fiber) + GORM + WebSocket** backend. SQLite by default (Postgres supported).

## Features (MVP + v1.1)
- Auth: register / login / JWT (24h), bcrypt hashing
- Servers with owner, invite links, members, auto `#general`
- Text channels (create / edit / delete)
- Real-time messaging over WebSocket: live send, edit, delete, typing indicator
- Message history with pagination (50/page, infinite scroll up)
- File attachments (images inline, others as links; 10MB limit)
- Friends: search, request, accept/reject, remove, online presence
- Notifications with unread badge, pushed live

## Run

### Backend (`server/`)
```bash
cd server
cp .env.example .env        # optional: edit JWT_SECRET, DB_DRIVER, etc.
go run .                    # listens on :3000, creates mitrachat.db
```
Postgres instead of SQLite: set `DB_DRIVER=postgres` and
`DB_DSN=postgres://user:pass@localhost:5432/mitrachat?sslmode=disable`.

### Frontend (`client/`)
```bash
cd client
npm install
npm run dev                 # http://localhost:5173 (proxies /api and /ws to :3000)
```

## Verify
- `cd server && go build ./...`
- `cd client && npm run type-check && npm run build`
- REST + WebSocket smoke tests pass (auth → server → channel → realtime message → friends).

## Architecture
- `server/internal/*` — one package per domain (auth, servers, channels, messages,
  attachments, friends, notifications) + `ws` (hub/rooms/broadcast), `database`,
  `middleware`, `config`, `utils`.
- WebSocket owns message persistence + broadcast (`ws.CreateAndBroadcast` etc.);
  HTTP message/attachment handlers reuse those helpers, so both paths stay consistent.
- Frontend: Pinia stores per domain, `ws/socket.ts` (auto-reconnect + event bus),
  axios client with JWT interceptor.
