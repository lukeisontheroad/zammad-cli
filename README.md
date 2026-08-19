# zammad-cli

[![CI](https://github.com/lukeisontheroad/zammad-cli/actions/workflows/test.yml/badge.svg)](https://github.com/lukeisontheroad/zammad-cli/actions/workflows/test.yml)
[![Release](https://img.shields.io/github/v/release/lukeisontheroad/zammad-cli)](https://github.com/lukeisontheroad/zammad-cli/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE.md)

`zammad` is a command-line client for [Zammad](https://zammad.org): a single
static binary, no runtime dependencies, configured via a config file or
environment variables.

```
$ zammad ticket list --owner me
ID     STATE  PRIORITY  TITLE                             GROUP    OWNER       UPDATED
57744  new    2 normal  Large-scale compute proposal      Sales    Jane Smith  2026-07-29 06:50
56893  open   2 normal  Payments page domain migration    Support  Jane Smith  2026-07-02 07:30
```

## Install

From source (Go ≥1.22):

```sh
go install github.com/lukeisontheroad/zammad-cli/cmd/zammad@latest
```

Homebrew (once the tap is published):

```sh
brew install lukeisontheroad/tap/zammad
```

Or grab a binary from the [releases page](https://github.com/lukeisontheroad/zammad-cli/releases).

## Authentication

Create an API token in Zammad under your avatar → **Profile** → **Token Access**
(direct URL: `https://<your-instance>/#profile/token_access`), checking the
`ticket.agent` permission — every command in this CLI works with that single
permission. If **Token Access** is missing from your profile menu, an admin
must enable it under **Admin → System → API → Token Access**, and your role
needs the `user_preferences.access_token` permission. Then:

```sh
zammad auth login
```

This validates the token and writes `~/.config/zammad/config.yml` (mode 0600).
Multiple instances are supported:

```sh
zammad auth login --name work --url https://support.example.com --token ...
zammad ticket list --instance work
```

For CI or one-off use, skip the config file entirely:

```sh
export ZAMMAD_URL=https://support.example.com
export ZAMMAD_TOKEN=your_token
```

Resolution order: `ZAMMAD_URL`+`ZAMMAD_TOKEN` → `--instance` flag →
`ZAMMAD_INSTANCE` env var → `default` entry in the config file.

## Usage

### Tickets

```sh
# List open tickets (default)
zammad ticket list
zammad ticket list --state all --limit 100
zammad ticket list --owner me --group Support

# View a ticket; ids and browser URLs both work
zammad ticket view 42
zammad ticket view 'https://support.example.com/#ticket/zoom/42'
zammad ticket view 42 --comments      # include articles
zammad ticket view 42 --web           # open in browser

# Create and modify
zammad ticket create --title "Printer on fire" --group Support \
    --customer jane@example.com --body "It is literally on fire."
zammad ticket update 42 --state "pending reminder" --priority "3 high"
zammad ticket reply 42 --body "Working on it."               # note (no email)
zammad ticket reply 42 --body "Vendor escalated." --internal  # internal note
zammad ticket reply 42 --body "Fix deployed." --email         # real email to customer,
                                                              # group signature auto-appended
zammad ticket reply 42 --body "FYI" --email --to jane@example.com --cc boss@example.com
zammad ticket reply 42 --body "See log." --attach ./error.log --time 15
zammad ticket reply 42 --body "Answer to all." --email --reply-all
zammad ticket reply 42 --body "**Fixed** in \`v1.2\`:\n- restart app" --email --markdown
zammad ticket close 42 --note "Fixed."
```

### Search

```sh
# Zammad's Elasticsearch query syntax, passed through verbatim
zammad ticket search 'state.name:open AND priority.name:"3 high"'
zammad ticket search 'tags:billing'

# Fuzzy customer/company search: matches organization name, customer email
# domains, and text mentions — works even without organization records
zammad ticket search --customer ACME
zammad ticket search --customer acme.com 'state.name:open'
zammad ticket list --customer ACME --state all

# Global search across tickets, users, and organizations
zammad search ACME
```

### Attachments, tags, history

```sh
zammad ticket attachments 42
zammad ticket attachments 42 --download --dir ./logs

zammad ticket tag list 42
zammad ticket tag add 42 billing urgent
zammad ticket tag remove 42 urgent

zammad ticket history 42              # who changed what, when
```

### Overviews and lookups

```sh
zammad overview list                  # your saved overviews with counts
zammad overview view my_assigned      # tickets in one overview

zammad user search jane
zammad org search acme
```

### Raw API escape hatch

Any endpoint, with authentication handled for you:

```sh
zammad api GET /api/v1/ticket_states
zammad api GET /api/v1/tickets/search -f query=tags:billing -f expand=true
zammad api PUT /api/v1/tickets/42 -f state=closed
```

### Scripting

Every command supports `-o json`, printing the raw API objects:

```sh
zammad ticket list -o json | jq '.[].title'
zammad ticket search --customer acme.com -o json | jq 'length'
```

Global flags: `--instance <name>`, `-o/--output table|json`, `--verbose`
(log HTTP requests to stderr).

## Development

```sh
make build   # build bin/zammad
make test    # go test ./...
make vet     # go vet ./...
make lint    # golangci-lint run
```

Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) for the commit
convention (Conventional Commits, drives automated changelog and semver
releases) and workflow.

Releases are fully automated: merging the release-please PR on `main` tags a
release; [goreleaser](https://goreleaser.com) then builds binaries and updates
the Homebrew package in `lukeisontheroad/homebrew-tap` (requires the
`HOMEBREW_TAP_TOKEN` repo secret).

## License

[MIT](LICENSE.md)

## Trademark notice

This is an independent community project. It is not affiliated with, endorsed
by, or sponsored by Zammad GmbH or the Zammad Foundation. "Zammad" is a
trademark of the Zammad Foundation; it is used here to describe
interoperability with the Zammad product.
