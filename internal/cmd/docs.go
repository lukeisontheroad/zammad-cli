package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// newDocsCmd prints the complete CLI reference in one shot. Intended for
// piping into LLM/agent context: `zammad docs` yields everything needed to
// operate the tool without further --help calls.
func newDocsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docs",
		Short: "Print the complete CLI reference (optimized for LLMs/agents)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprint(out, docsHeader)
			printCmdDocs(out, cmd.Root())
			fmt.Fprint(out, docsFooter)
			return nil
		},
	}
}

const docsHeader = `# zammad CLI reference

Command-line client for the Zammad helpdesk. All commands are non-interactive
(except a bare "auth login") and print to stdout; errors go to stderr with
exit code 1, success is exit code 0.

## Setup

Authentication (pick one):
- Config file: run "zammad auth login --url <URL> --token <TOKEN>" once.
- Environment: set ZAMMAD_URL and ZAMMAD_TOKEN (overrides the config file).
The token is a Zammad API access token (Profile > Token Access) with the
"ticket.agent" permission; that single permission covers every command here.

## Conventions

- Ticket arguments accept a plain id ("42", "#42") or a browser URL
  ("https://host/#ticket/zoom/42").
- Every read command supports "-o json", printing the raw Zammad API objects
  (stable field names, machine-parseable). Prefer it when consuming output
  programmatically; the default table view truncates long values.
- Names are used instead of ids where Zammad allows it: --group, --state,
  --priority take names ("Support", "open", "3 high"); --customer takes an
  email address.
- "--verbose" logs each HTTP request to stderr for debugging.

## Search query syntax (ticket search, ticket list filters)

Zammad uses Elasticsearch query syntax. Useful fields:
  state.name:open              priority.name:"3 high"     group.name:Support
  owner.login:jdoe             customer.email:*acme.com   organization.name:ACME
  tags:billing                 number:20001               title:printer
Operators: AND, OR, parentheses, quoting for values with spaces, * wildcards,
date ranges like created_at:[2026-01-01 TO 2026-02-01].
Bare terms search full text (subject + body).
The "--customer TERM" flag on ticket list/search builds a fuzzy clause matching
organization name OR customer email wildcard OR full text, useful when no
organization records exist.

## Commands

`

const docsFooter = `
## Raw API access

"zammad api METHOD /api/v1/..." reaches any endpoint with authentication
handled. GET/DELETE: -f key=value pairs become query parameters (repeatable).
Other methods: pairs form a flat JSON body. Responses print as JSON.
Endpoint reference: https://docs.zammad.org/en/latest/api/intro.html
`

// printCmdDocs walks the command tree and prints usage, description,
// examples, and flags for every runnable command.
func printCmdDocs(w io.Writer, c *cobra.Command) {
	if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
		return
	}
	if c.Runnable() && c.Name() != "docs" {
		fmt.Fprintf(w, "### %s\n", c.CommandPath())
		fmt.Fprintf(w, "Usage: %s\n", c.UseLine())
		desc := c.Long
		if desc == "" {
			desc = c.Short
		}
		if desc != "" {
			fmt.Fprintln(w, desc)
		}
		if c.Example != "" {
			fmt.Fprintf(w, "Examples:\n%s\n", c.Example)
		}
		flags := c.NonInheritedFlags()
		if flags.HasAvailableFlags() {
			fmt.Fprintf(w, "Flags:\n%s", flags.FlagUsages())
		}
		fmt.Fprintln(w)
	}
	for _, sub := range c.Commands() {
		printCmdDocs(w, sub)
	}
}
