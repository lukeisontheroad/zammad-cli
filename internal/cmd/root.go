// Package cmd implements the zammad command tree.
package cmd

import (
	"fmt"

	"github.com/lukeisontheroad/zammad-cli/internal/api"
	"github.com/lukeisontheroad/zammad-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	flagInstance string
	flagOutput   string
	flagVerbose  bool
)

func Execute(version string) error {
	return newRootCmd(version).Execute()
}

// newRootCmd builds the full command tree (also used by tests).
func newRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "zammad",
		Short: "Work with Zammad tickets from the command line",
		Long: `Work with Zammad tickets from the command line.

Authentication: run "zammad auth login" once, or set ZAMMAD_URL and
ZAMMAD_TOKEN environment variables (token needs the "ticket.agent"
permission — sufficient for every command).

Ticket arguments accept plain ids ("42", "#42") or browser URLs
("https://host/#ticket/zoom/42"). Every read command supports "-o json"
for machine-readable output (raw Zammad API objects).

Run "zammad docs" for the complete reference in one page (recommended
for LLM/agent use).`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&flagInstance, "instance", "", "config instance (profile) to use")
	root.PersistentFlags().StringVarP(&flagOutput, "output", "o", "table", `output format: "table" (human) or "json" (raw API objects, for scripting)`)
	root.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "log HTTP requests to stderr")

	root.AddCommand(newAuthCmd())
	root.AddCommand(newTicketCmd())
	root.AddCommand(newSearchCmd())
	root.AddCommand(newOverviewCmd())
	root.AddCommand(newUserCmd())
	root.AddCommand(newOrgCmd())
	root.AddCommand(newAPICmd())
	root.AddCommand(newDocsCmd())

	return root
}

// newClient resolves the active instance and returns an API client for it.
func newClient() (*api.Client, error) {
	_, url, token, err := config.Resolve(flagInstance)
	if err != nil {
		return nil, err
	}
	c, err := api.New(url, token)
	if err != nil {
		return nil, err
	}
	c.Verbose = flagVerbose
	return c, nil
}

func validateOutput() error {
	if flagOutput != "table" && flagOutput != "json" {
		return fmt.Errorf("invalid --output %q (want table or json)", flagOutput)
	}
	return nil
}
