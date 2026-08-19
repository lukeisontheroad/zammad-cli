# Security Policy

## Supported versions

Only the latest release receives security fixes.

## Reporting a vulnerability

Please do not open public issues for security problems.

Report vulnerabilities via
[GitHub private vulnerability reporting](https://github.com/lukeisontheroad/zammad-cli/security/advisories/new)
on this repository. You will receive a response within a few days.

## Scope notes

- The CLI stores API tokens in `~/.config/zammad/config.yml` with `0600`
  permissions, or reads them from environment variables. It never sends the
  token anywhere except the configured Zammad instance (Authorization header).
- `--verbose` logs request URLs (not the token) to stderr.
- Dependencies are scanned with govulncheck in CI and updated via Dependabot.
