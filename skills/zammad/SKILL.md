---
name: zammad
description: >
  Work with Zammad helpdesk tickets via the zammad CLI: list, search, view,
  create, and update tickets, send email replies, manage tags and
  attachments. Use when the user asks about their Zammad tickets, helpdesk
  queue, customer support tickets, or wants to reply to / triage tickets
  from the terminal.
---

# Operating the zammad CLI

## Setup check

Run `zammad auth status`. If it fails, the user must authenticate first:
`zammad auth login` (interactive), or set `ZAMMAD_URL` + `ZAMMAD_TOKEN`.
Do not attempt to create API tokens yourself.

## Reference

Run `zammad docs` once — it prints the complete command reference (all
commands, flags, and the search query syntax) in a single page designed for
LLM consumption. Prefer it over multiple `--help` calls.

## Rules

- Use `-o json` whenever you consume output programmatically; tables
  truncate long values. JSON output is the raw Zammad API objects.
- Ticket arguments accept ids (`42`, `#42`) and browser URLs
  (`https://host/#ticket/zoom/42`) interchangeably.
- Search uses Elasticsearch syntax (`state.name:open AND tags:billing`).
  For "tickets from company X" use the fuzzy flag instead of hand-building
  queries: `zammad ticket search --customer X` — it matches organization
  names, customer email domains, and text mentions.
- Read operations are safe to run freely. Write operations (`create`,
  `update`, `close`, `reply`, `tag add/remove`) change the helpdesk for
  the whole team — confirm with the user before running them unless they
  explicitly asked for the exact change.
- `reply` posts a note by default (no email leaves the system).
  `reply --email` sends a REAL email to the customer (signature appended
  automatically; `--reply-all` CCs the thread) — always confirm body and
  recipients with the user first.
- `zammad api <method> <path>` reaches any Zammad API endpoint when no
  dedicated command exists.
