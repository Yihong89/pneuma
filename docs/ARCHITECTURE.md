# pneuma: architecture

This document describes how pneuma works inside: how it identifies devices, what it stores, the HTTP API the web app uses, and the rules it enforces. For a user-level overview, see the [README](../README.md).

## Overview

```
                       ┌──────────────────────── pneuma process ───────────────────────┐
  browser ── HTTPS ──▶ │ tsnet listener :443 ─▶ identify (WhoIs) ─▶ cross-origin check │
  (any tailnet device) │                                  │                             │
                       │                                  ▼                             │
                       │        HTTP API (internal/server) ───▶ store (SQLite + files)  │
                       │                 │                                              │
                       │                 └──▶ hub ── Server-Sent Events ──▶ browsers     │
                       │                                                                │
                       │ retention loop (hourly): expire files, delete old messages     │
                       └────────────────────────────────────────────────────────────────┘
```

The whole thing is one Go binary:

| Package | Responsibility |
|---|---|
| `cmd/pneuma` | Flags and env vars; starts `tsnet` (or a plain localhost listener with `-dev`); runs the retention loop |
| `internal/ident` | Works out who is calling, and lists the tailnet's devices |
| `internal/store` | SQLite schema and migrations, chats, messages, files on disk, settings, device aliases |
| `internal/hub` | Fans events out to open SSE streams; tracks which devices have pneuma open |
| `internal/server` | HTTP routes, access checks, uploads and downloads, settings, retention |
| `web` | The browser app: plain HTML, CSS and JS, embedded with `embed.FS` |

## Joining the tailnet

`tsnet.Server` runs a userspace Tailscale node inside the process. It needs no TUN device, no root and no Tailscale installation on the host. Its state (node key and so on) lives in `<data>/tailscale`. pneuma listens on:

- `:443` with `ListenTLS`. The certificate for `pneuma.<tailnet>.ts.net` is obtained through Tailscale, which requires MagicDNS and HTTPS to be enabled for the tailnet.
- `:80`, which only redirects to HTTPS.

Nothing is exposed outside the tailnet.

## Identity

Every request passes through `authenticate` (`internal/server/server.go`), which calls `ident.Identifier.Identify`:

- **Tailscale mode:** `LocalClient.WhoIs(remoteAddr)` returns the peer's node and user profile. pneuma builds a `store.Sender` from it:
  - `ID`: the node's **StableNodeID**. It never changes for that machine, and access control is based on it.
  - `Device`: the MagicDNS name (`laptop`), used for display.
  - `OS` and `User` (login name).

  Requests are rejected with `403` when the node is **tagged** (unless `PNEUMA_ALLOW_TAGGED`) or when `PNEUMA_ALLOWED_USERS` is set and the login isn't in it.
- **Dev mode:** a fake identity. `?as=<name>` picks the device name and the ID is `dev:<name>`.

The device list (`GET /api/devices`) comes from `LocalClient.Status()`, filtered by the same rules, so pneuma never shows a device that couldn't actually use it.

Writes also go through Go's `http.CrossOriginProtection`. A request whose `Sec-Fetch-Site` or `Origin` says it came from another site is refused, so a page open in one of your browsers can't act with that device's identity.

## Data model

SQLite (`<data>/pneuma.db`, WAL mode, a single connection), plus one file per upload in `<data>/files/<random id>`.

```
chats     id, name, created_at, members        -- members: newline-separated stable IDs; '' = public
messages  id, chat_id, created_at, device, device_os, user, kind, body, file_id
files     id, name, size, mime, sha256
settings  key, value                            -- retention_days, file_retention_days, max_upload
aliases   device, alias                         -- display names, keyed by MagicDNS name
```

- `kind` is `text`, `file` or `expired`. An expired message is a file message whose file was removed by retention; `body` keeps the original name so the history still shows what was there.
- Migrations run on start-up and are additive: chats were added to an older single-room schema, and `members` was added later. Existing data is preserved.
- There is always at least one public chat. Deleting the last one creates a fresh `general`.

### Uploads

`POST /api/chats/{chat}/files` reads the multipart body part by part. Each file is streamed to disk through a SHA-256 hasher, with a `LimitReader` enforcing the max upload size, so a 4 GB upload never sits in memory. Non-file fields are capped at 1 MB in total. The Android Share Target (`POST /share`) uses the same path.

### Downloads

`GET /f/{id}/{name}` first finds the file's chat and checks the caller may see it. The file is then served with `http.ServeContent` (range requests work, so videos can seek) and these headers:

- `Content-Security-Policy: sandbox; default-src 'none'; …`
- `X-Content-Type-Options: nosniff`
- `Content-Disposition`: `inline` only for images (except SVG), video, audio, PDF and plain text; `attachment` for everything else, or when `?dl=1` is given.

## Private chats

A chat with an empty `members` list is public. Otherwise only the listed stable IDs can see it:

- `GET /api/chats` returns only the chats visible to the caller.
- Every chat-scoped route (messages, files, rename, delete, members), message deletion and file download loads the chat and checks `Chat.Visible(caller.ID)`. Chats the caller can't see answer `404`, the same as chats that don't exist, so their existence doesn't leak.
- Members can only be added if they are current tailnet devices (`knownDevice`), and the creator is always included.
- Events about a private chat are published with an *audience* (its member IDs). The hub only delivers them to SSE streams opened by those devices.

## Live updates

`GET /api/events` is a Server-Sent Events stream. On connect the hub registers the device for presence. It sends a `retry` hint and a ping every 25 s. A subscriber that can't keep up (its 64-event buffer is full) is disconnected rather than allowed to block everyone; its browser reconnects and resyncs.

| Event | Payload | Audience |
|---|---|---|
| `message` | the new message | chat members |
| `delete` | `{id, chat_id}` | chat members |
| `chat` | a created, renamed or extended chat | chat members |
| `chat_delete` | `{id}` | chat members |
| `presence` | `{active: [device names]}` | everyone |
| `aliases` | `{device: alias}` | everyone |
| `settings` | shared settings | everyone |
| `resync` | `{changed}` after retention ran | everyone |

On every (re)connect the client re-fetches the chat list and the newest page of the current chat. It reconciles them with what it already shows, so messages and deletions missed while offline heal themselves.

## Retention

`RunRetention` runs at start-up, every hour, and whenever a retention setting changes:

1. **Files** older than `file_retention_days` (default **30**) are deleted from disk; their messages become `expired`.
2. **Messages** older than `retention_days` (default **0**, meaning never) are deleted together with any file.

## HTTP API

All routes require a tailnet identity, and all request and response bodies are JSON unless noted.

| Method & path | Purpose |
|---|---|
| `GET /api/me` | The caller (`sender`) and the current `max_upload` |
| `GET /api/devices` | Tailnet devices: `id`, `name`, `os`, `online`, `active` (pneuma open) |
| `PUT /api/devices/{name}/alias` | `{alias}`; set or clear (`""`) a display name |
| `GET /api/settings` | `{settings, usage, aliases}` |
| `PATCH /api/settings` | Any of `{retention_days, file_retention_days, max_upload}` |
| `GET /api/chats` | Chats visible to the caller, most recently active first |
| `POST /api/chats` | `{name, members?: [stable IDs]}`; members makes it private |
| `PATCH /api/chats/{chat}` | `{name}` |
| `DELETE /api/chats/{chat}` | Deletes the chat, its messages and its files |
| `POST /api/chats/{chat}/members` | `{device: stable ID}`; private chats only |
| `GET /api/chats/{chat}/messages?before=&limit=` | A page of messages, oldest first (`limit` ≤ 200) |
| `POST /api/chats/{chat}/messages` | `{text}`, up to 1 MB |
| `GET /api/chats/{chat}/files` | File messages of the chat, newest first |
| `POST /api/chats/{chat}/files` | `multipart/form-data`, one or more `file` parts |
| `DELETE /api/messages/{id}` | Deletes a message and its file |
| `GET /api/events` | Server-Sent Events stream (see above) |
| `GET /f/{id}/{name}[?dl=1]` | Download or view a file |
| `POST /share` | Web Share Target (multipart `title`, `text`, `url`, `file`); goes to the most recent chat |

## Web app

`web/` is served as static files with `Cache-Control: no-cache`, so a redeploy shows up on the next load. A service worker caches the app shell (never the API or files) so the PWA starts instantly.

The app is a single ES module (`app.js`) with no framework:

- Message nodes are cached by ID. The list is rebuilt from them on each change, which keeps media elements alive and makes grouping and day separators simple.
- All user content is inserted as text nodes. `innerHTML` is only ever used with constant markup (icons).
- Preferences that belong to one device (language, text size, Enter to send, notifications, animations) live in `localStorage`. Everything shared lives on the server.
- The logo is drawn as inline SVG pixels. Its iris is a separate layer, clipped to the white of the eye, so it can follow the pointer.
