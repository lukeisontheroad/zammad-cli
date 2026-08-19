# zammad-cli

Go CLI for the Zammad helpdesk REST API. Single static binary, cobra command
tree, no SDK dependency — the API client is hand-rolled on `net/http`.

## Commands

```sh
make build          # build bin/zammad (version via ldflags)
make test           # go test ./...
make vet            # go vet ./...
make lint           # golangci-lint run (config: .golangci.yml, v2 format)
go test ./internal/api/    # single package
```

## Architecture

- `cmd/zammad/main.go` — entrypoint; `version` injected via `-ldflags -X main.version=`
- `internal/cmd/` — one file per command group (root, auth, ticket, extras, api).
  All commands: resolve client via `newClient()`, respect `flagOutput`
  (`table`|`json`), render via `internal/output`. JSON output prints the raw
  API response (`Ticket.Raw`), never re-marshaled structs.
- `internal/api/` — Zammad REST client. `client.go` holds `Do()` (auth header,
  JSON, error mapping); resource methods in `tickets.go`, `articles.go`,
  `users.go`, `extras.go` (tags, history, overviews, global search, attachments).
- `internal/config/` — `~/.config/zammad/config.yml` (0600), multi-instance.
  Resolution order: `ZAMMAD_URL`+`ZAMMAD_TOKEN` env pair → `--instance` flag →
  `ZAMMAD_INSTANCE` env → `default` key in config.
- `internal/output/` — tabwriter tables + JSON printer.

Tests use `httptest.Server` fixtures (`internal/api/client_test.go`), no live
instance required.

## Zammad API facts (verified against docs.zammad.org)

- Auth header: `Authorization: Token token={token}`. Everything the CLI does
  works with the `ticket.agent` permission alone — keep it that way; degrade
  gracefully on 403 instead of requiring admin permissions.
- `expand=true` on ticket endpoints resolves relations to strings
  (`"state": "open"`, `"group": "Sales"`, `"owner": "<login>"`). The CLI
  always requests it; `Ticket.Raw` keeps the full JSON for `-o json`.
- Owner logins may be opaque (UUIDs on some instances) — table output resolves
  owner names via `GET /users/{id}` (`ownerNames` in `internal/cmd/ticket.go`);
  `owner_id <= 1` means unassigned.
- Search: `GET /api/v1/tickets/search?query=` with Elasticsearch syntax.
  No documented sort/limit params — paginate with `page`/`per_page` until a
  short page (see `SearchTickets`).
- Writes accept human-readable relation names (`group`, `state`, `priority`,
  `customer`). `guess:{email}` auto-creates unknown customers but 422s for
  existing ones on some instances — `ticket create` therefore sends the plain
  email first and retries with `guess:` only on a 422 customer-lookup error
  (`isUnknownCustomer`). A ticket `PUT` with a nested `article` object creates
  a new article — that is how `reply`/`close --note` work.
- `reply --email` recipient mirrors the UI: sender of the customer's last
  email article (Reply-To over From, `replyRecipient`), falling back to the
  ticket customer. `--reply-all` CCs the source email's To+Cc minus the
  instance's own inboxes (`/api/v1/email_addresses`, agent-accessible), the
  agent, and the To recipient. `--markdown` renders the body via goldmark
  (GFM). `--attach` base64-encodes files (`loadAttachments`); `--time` sets
  `time_unit` for instances enforcing time accounting.
- Email subjects: do NOT build "[Ticket#...]" hooks client-side. The server's
  subject_build strips and re-applies the hook on every outgoing mail per the
  instance's ticket_hook_position setting (may be "none"); it also falls back
  to the ticket title when the article has no subject. Send the user's subject
  verbatim or omit it.
- Email signatures are rendered by the Zammad web UI, never the server —
  API-created email articles go out bare. `reply --email` therefore fetches
  the group's signature (`/api/v1/signatures`, agent-accessible), substitutes
  `#{user.x}` placeholders from `/users/me`, and appends it as HTML
  (`emailBody` in `internal/cmd/ticket.go`); `--no-signature` opts out.
- Quirks: attachment upload field is `mime-type` (hyphen); tag remove is a
  `DELETE` with a JSON body; `/api/v1/overviews` is admin-only (use
  `/api/v1/ticket_overviews`, agent-accessible, response shape
  `{assets, index: {tickets: [{id}]}}`); global `/api/v1/search` returns
  `{result: [{type, id}], assets: {...}}`.
- Ticket delete is admin-only and irreversible — deliberately not exposed.

## Conventions

- New commands: constructor `newXxxCmd() *cobra.Command` in `internal/cmd/`,
  registered in `root.go` (or under `newTicketCmd()` for ticket subcommands).
  Always support `-o json`; call `validateOutput()` first in RunE.
- Query building: escape user input with `quoteQueryValue`; fuzzy
  customer/company matching lives in `customerQuery` (org name OR email
  wildcard OR fulltext) — extend that rather than adding parallel logic.
- Ticket id arguments go through `parseTicketID` (accepts `42`, `#42`, and
  zoom URLs) — reuse it for any new ticket-taking command.
- Errors: return them; root command has `SilenceUsage` — no manual usage dumps.

## Commits

Conventional Commits, enforced by commitlint in CI and consumed by
release-please for versioning + CHANGELOG (see CONTRIBUTING.md).
`feat:` = minor, `fix:`/`perf:` = patch, `!`/`BREAKING CHANGE:` = major;
`docs:`/`test:`/`ci:`/`chore:`/`refactor:` don't bump. Imperative, lowercase,
no trailing period; optional scope = command or package name
(`feat(ticket): ...`). Write descriptions for the changelog reader.

## Release

Fully automated: release-please watches `main`, maintains a release PR with
the next semver version + CHANGELOG.md from conventional commits; merging it
creates the `v*` tag, which triggers goreleaser
(`.github/workflows/release.yml`): darwin/linux/windows binaries + Homebrew
formula pushed to `lukeisontheroad/homebrew-tap` (needs `HOMEBREW_TAP_TOKEN`
secret — PAT with repo scope on the tap repo). Never tag manually.
