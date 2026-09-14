# gitlab-tg-notify

A small Go service that turns GitLab issue webhooks into Telegram
notifications — including confidential issues and Service Desk
email-to-issue — and keeps every follow-up (comment, update, close, reopen)
threaded as a reply to the message that first announced the issue. Optional
Telegram forum "Topics" support routes each GitLab project to its own topic
in the same group.

Full design and rationale: [`plan/gitlab-telegram-bot-prd.md`](plan/gitlab-telegram-bot-prd.md).
Agent/contributor quick-reference: [`AGENTS.md`](AGENTS.md).

## Features

- **One-way GitLab → Telegram bridge.** Listens for GitLab's `Issue Hook`,
  `Confidential Issue Hook`, `Note Hook`, and `Confidential Note Hook`
  webhook events over HTTP and forwards them as Telegram messages.
- **Reply-threading.** The message announcing a new issue is remembered
  (SQLite); every later event for that issue (comment/update/close/reopen)
  is sent as a Telegram reply to that original message, so a whole issue's
  activity reads as one thread.
- **Per-project allow-list.** Only GitLab projects explicitly listed in
  `config.yaml` are processed — anything else is silently ignored, so a
  stray webhook from an unlisted project can't leak into the wrong group.
- **Optional forum Topics.** With `use_topics: true`, each allow-listed
  project is pinned to its own Telegram forum topic
  (`telegram_message_thread_id`) instead of all projects sharing one
  timeline.
- **Confidential issue support.** Confidential issues and their comments are
  forwarded like any other issue — GitLab's `Confidential Issue Hook` /
  `Confidential Note Hook` events are handled explicitly, so treat the
  destination Telegram group/topic as equally sensitive.
- **Per-comment time tracking.** If a comment logs time via GitLab's
  `/spend` quotes, the notification shows exactly how much time *that
  comment* added (e.g. `⏱ Time spent: 1h 30m`), not the issue's running
  total — comments with no time logged show `⏱ Time spent: 0m`.
- **HTML-safe, size-capped messages.** Titles, actor names, descriptions,
  and comment bodies are HTML-escaped for Telegram's `parse_mode: HTML`;
  messages over Telegram's 4096-character cap are truncated with a
  "… (read more on GitLab)" link back to the issue.
- **Webhook authentication.** Every request is checked against a shared
  secret (`X-Gitlab-Token`) using a constant-time comparison.
- **Fail-fast configuration.** The service refuses to start if
  `use_topics: true` but a listed project has no
  `telegram_message_thread_id` configured — this is caught at startup, not
  at the first missed notification.
- **Fast webhook responses.** The HTTP handler validates and returns `200
  OK` immediately; the actual Telegram send + database write happen off the
  request goroutine, so GitLab's webhook delivery never times out waiting on
  Telegram.
- **Resilient delivery.** Telegram sends are retried up to 3 times with
  backoff (2s / 5s / 10s) before giving up and logging the failure.
- **Graceful degradation.** If the root message for an issue can't be found
  (e.g. the bot was redeployed and lost its database, or the event arrived
  out of order), the follow-up is sent as a standalone message instead of
  being dropped.
- **Pure-Go, CGO-free build.** Uses `modernc.org/sqlite` (pure Go SQLite
  driver) so the binary and Docker image need no C toolchain.

## Supported events

| GitLab event | Telegram message |
|---|---|
| Issue opened | 🆕 New issue announcement — this becomes the thread's root message |
| Issue updated | ✏️ Update notice, replied to the root message |
| Issue commented | 💬 Comment text + per-comment time spent, replied to the root message |
| Issue closed | ✅ Closed notice, replied to the root message |
| Issue reopened | 🔁 Reopened notice, replied to the root message |

Comments on merge requests, commits, or snippets are ignored (issue
comments only). Merge request, pipeline, and wiki events are out of scope
for this version — see [Non-goals](#non-goals-and-future-work).

### Example message

An issue comment where `/spend 1h30m` was used:

```
💬 Login page throws 500 on submit
📌 Issue ID : #128
👤 Commented by: Jane Doe
⏱ Time spent: 1h 30m
🔗 https://gitlab.example.com/backend/api/-/issues/128

Reproduced on staging — looks like the session cookie is expiring
mid-request. Will patch tonight.
```

A plain comment with no time logged ends with `⏱ Time spent: 0m` instead.
Non-comment events (opened/updated/closed/reopened) don't show a time-spent
line at all.

## Architecture

```
GitLab (issue/note webhook)
        │  HTTP POST, X-Gitlab-Token
        ▼
internal/webhook   — verify token, parse payload, allow-list filter,
                      reply-threading decisions
        │
        ├──► internal/store    — SQLite: issue → root message ID,
        │                        per-issue time-tracking baseline
        │
        └──► internal/telegram — HTML message formatting + gotgbot send
                                  (with retry/backoff)
```

- `cmd/bot` wires everything together and runs the HTTP server with
  graceful shutdown (`SIGINT`/`SIGTERM`).
- `internal/model` defines `NotificationEvent`, the transport-agnostic type
  that decouples `webhook` (GitLab-shaped input) from `telegram`
  (Telegram-shaped output).
- `internal/config` loads and validates `config.yaml`.
- `migrations` holds the embedded SQL schema (`issue_threads`,
  `issue_time_tracking`), applied automatically on startup.

## Project layout

```
cmd/bot/                  entry point: config → store → telegram client → HTTP server
internal/config/          config.yaml loading, defaults, validation
internal/model/           NotificationEvent (shared event shape)
internal/store/           SQLite persistence (reply-threading, time tracking)
internal/telegram/        message formatting + gotgbot client wrapper
internal/webhook/         HTTP handler: auth, payload parsing, dispatch
migrations/               embedded SQL schema
config.example.yaml       config template (copy to config.yaml)
Dockerfile, docker-compose.yml
```

## Configuration

Copy [`config.example.yaml`](config.example.yaml) to `config.yaml` and fill
in real values — `config.yaml` is gitignored and must never be committed.

| Key | Required | Default | Notes |
|---|---|---|---|
| `server.listen_addr` | no | `0.0.0.0:8080` | HTTP listen address |
| `server.webhook_path` | no | `/webhook/gitlab` | Path GitLab should POST to |
| `telegram.bot_token` | **yes** | — | From [@BotFather](https://t.me/BotFather) |
| `telegram.chat_id` | **yes** | — | Target group/supergroup chat ID (negative number) |
| `telegram.use_topics` | no | `false` | Route each project to its own forum topic |
| `gitlab.base_url` | no | — | Informational; not required for webhook processing |
| `gitlab.webhook_secret` | **yes** | — | Shared secret GitLab sends as `X-Gitlab-Token` |
| `database.path` | no | `/app/data/bot.db` | SQLite file path |
| `logging.level` | no | `info` | `debug` \| `info` \| `warn` \| `error` |
| `logging.format` | no | `text` | `text` \| `json` |
| `projects[].gitlab_project_id` | **yes** | — | GitLab numeric project ID (allow-list) |
| `projects[].gitlab_project_path` | **yes** | — | `namespace/project`, for logging only |
| `projects[].telegram_message_thread_id` | conditional | — | **Required per-project if `use_topics: true`** |

Startup fails fast with a descriptive error if `use_topics: true` and any
listed project is missing `telegram_message_thread_id`, rather than silently
dropping that project's notifications later.

## Quick start

1. Copy the example config and fill in real values:
   ```sh
   cp config.example.yaml config.yaml
   ```
2. Create a Telegram bot via [@BotFather](https://t.me/BotFather), add it to
   your group (as admin, if using topics), and get the group's chat ID.
3. In GitLab, add a project webhook pointing at
   `https://<your-host>/webhook/gitlab` with the same secret as
   `gitlab.webhook_secret`, and enable the **Issues** and **Comments**
   triggers (see the full walkthrough in the PRD, §16).
4. Run locally:
   ```sh
   go run ./cmd/bot
   ```
   or with Docker:
   ```sh
   docker compose up --build
   ```

`CONFIG_PATH` overrides the config file location (default
`/app/config.yaml`); the Docker Compose setup mounts `./config.yaml` there
read-only and persists the SQLite file in a named volume.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

`CGO_ENABLED=0` is required in environments without a C toolchain (the
SQLite driver, `modernc.org/sqlite`, is pure Go and doesn't need one):

```sh
CGO_ENABLED=0 go build ./...
```

## Non-goals and future work

Out of scope for this version (see PRD §3, §17):

- Merge request, pipeline, and wiki events.
- Any Telegram → GitLab action — this is a strictly one-way flow.
- Automatic Telegram topic creation (topics must be created manually and
  mapped in `config.yaml`).

## License

[MIT](LICENSE)
