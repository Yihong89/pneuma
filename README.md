# pneuma

**A private chat and file drop for your own devices, over your [Tailscale](https://tailscale.com) tailnet.**

Type, paste or drop anything on one machine and it shows up on all the others: snippets, links, screenshots, PDFs, videos. Click a message to copy it. That's it.

![pneuma](docs/screenshot.png)

- **No accounts, no passwords.** pneuma joins your tailnet as its own node and asks Tailscale who is connecting, so each message is signed with the device that sent it (`laptop`, `mac-mini`, `pixel`…).
- **Private.** Nothing listens on the internet or your LAN: pneuma can only be reached from inside your tailnet, over HTTPS with a certificate Tailscale issues for it.
- **Copy & paste first.** `Ctrl+V` a screenshot or files to send them, drag and drop anywhere, click any message (or code block) to copy it, "copy image" on pictures.
- **History** that survives reboots, live updates on every device, infinite scroll.
- **Installable (PWA)** on Android, macOS, Windows and Linux. On Android pneuma appears in the system **Share** menu.
- **One small binary**, no CGO, no external services. SQLite and files on disk.

## How it works

```
 laptop ─┐                         ┌──────────────────────────┐
 mac ────┼── WireGuard (tailnet) ──▶  pneuma  (tsnet node)    │
 phone ──┘                         │  WhoIs → "who is this?"  │
                                   │  SQLite + files on disk  │
                                   └──────────────────────────┘
```

pneuma embeds Tailscale through [`tsnet`](https://tailscale.com/kb/1244/tsnet). It gets its own name (`https://pneuma.<your-tailnet>.ts.net`), its own TLS certificate, and for every request it can ask the tailnet which node and user is on the other end.

## Run it

You need a Tailscale account and a machine that stays on (a home server, a NAS, a small VM).

### Docker Compose

```sh
git clone https://github.com/ericpastorm/pneuma
cd pneuma
docker compose up -d --build
docker logs -f pneuma   # first run: open the login URL it prints
```

Or skip the login URL with an [auth key](https://login.tailscale.com/admin/settings/keys): `TS_AUTHKEY=tskey-auth-… docker compose up -d --build`.

Then, in the Tailscale admin console, **disable key expiry** for the `pneuma` machine so it doesn't drop off in 180 days.

Open `https://pneuma.<your-tailnet>.ts.net` from any of your devices. The first HTTPS request can take a few seconds while the certificate is issued.

### Binary

```sh
go install github.com/ericpastorm/pneuma/cmd/pneuma@latest
pneuma            # data goes to ~/.config/pneuma
```

## Configuration

| Env var | Flag | Default | |
|---|---|---|---|
| `PNEUMA_HOSTNAME` | `-hostname` | `pneuma` | Node name on the tailnet |
| `PNEUMA_DATA_DIR` | `-data` | `~/.config/pneuma` (`/data` in Docker) | Messages, files and tailnet state |
| `PNEUMA_MAX_UPLOAD` | `-max-upload` | `4GB` | Max size per upload |
| `PNEUMA_ALLOWED_USERS` | `-allowed-users` | everyone | Comma-separated tailnet logins allowed in |
| `PNEUMA_ALLOW_TAGGED` | `-allow-tagged` | `false` | Let tagged nodes (servers, CI) in |
| `TS_AUTHKEY` | | | Tailscale auth key for unattended setup |

## Security model

- Reachable only through your tailnet; there are no ports to publish.
- Every request is identified with Tailscale's WhoIs. **Tagged nodes are rejected by default**, because they belong to infrastructure, not people. You can restrict access further with `PNEUMA_ALLOWED_USERS` and with your tailnet ACLs.
- Uploaded files are served with `Content-Security-Policy: sandbox` and `nosniff`. Anything that could run script (HTML, SVG…) is always downloaded, never rendered.
- Anyone allowed in can read and delete all messages. pneuma is built for one person's devices (or a household), not for multi-user chat.

## Add it to your desktop

- **Android / Chrome:** open the URL and tap *Install app*. pneuma then shows up in the Share menu.
- **macOS:** Safari → *File → Add to Dock*, or *Install* in Chrome.
- **Linux (Omarchy / Hyprland):** `omarchy-launch-webapp https://pneuma.<your-tailnet>.ts.net`, or bind it to a key.

## Development

```sh
go run ./cmd/pneuma -dev          # http://localhost:8080, no Tailscale needed
```

In dev mode, add `?as=mac-mini` to the URL to pretend to be another device; two tabs make a conversation. The frontend is plain HTML/CSS/JS in `web/`, embedded in the binary, with no build step.

```sh
go test ./...
```

## En español

pneuma es un chat privado y una bandeja de archivos entre tus propios equipos, a través de tu tailnet de Tailscale. Escribes, pegas (`Ctrl+V`) o arrastras algo en un equipo y aparece en todos los demás. Un clic en cualquier mensaje lo copia. No hay cuentas ni contraseñas: Tailscale ya sabe qué equipo se conecta. La interfaz sale en español automáticamente si tu navegador está en español.

## License

[MIT](LICENSE)
