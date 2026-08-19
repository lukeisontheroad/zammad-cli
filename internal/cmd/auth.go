package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/lukeisontheroad/zammad-cli/internal/api"
	"github.com/lukeisontheroad/zammad-cli/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Log in to a Zammad instance and inspect auth status",
	}
	cmd.AddCommand(newAuthLoginCmd(), newAuthStatusCmd(), newAuthLogoutCmd())
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var urlFlag, tokenFlag, nameFlag string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate against a Zammad instance",
		Long: `Authenticate against a Zammad instance with an API access token.

Create a token in Zammad under your avatar > Profile > Token Access
(direct URL: https://<your-instance>/#profile/token_access).

Required token permissions:
  - ticket.agent            read, search, and modify tickets
  - user_preferences (opt.) only needed for some profile endpoints

If "Token Access" is missing from your profile menu, an admin must
enable it under Admin > System > API > "Token Access", and your role
needs the "user_preferences.access_token" permission.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := bufio.NewReader(cmd.InOrStdin())

			url := urlFlag
			if url == "" {
				fmt.Fprint(cmd.OutOrStdout(), "Zammad URL (e.g. https://support.example.com): ")
				line, err := in.ReadString('\n')
				if err != nil {
					return err
				}
				url = strings.TrimSpace(line)
			}
			if url == "" {
				return fmt.Errorf("URL is required")
			}
			if !strings.Contains(url, "://") {
				url = "https://" + url
			}

			token := tokenFlag
			if token == "" {
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "\nCreate an API token here: %s/#profile/token_access\n", strings.TrimRight(url, "/"))
				fmt.Fprintln(out, `Check the "ticket.agent" permission when creating it.`)
				fmt.Fprintln(out, `(Menu missing? An admin must enable Admin > System > API > "Token Access",`)
				fmt.Fprintln(out, ` and your role needs the "user_preferences.access_token" permission.)`)
				fmt.Fprint(out, "\nAPI token (input hidden): ")
				if term.IsTerminal(int(os.Stdin.Fd())) {
					b, err := term.ReadPassword(int(os.Stdin.Fd()))
					fmt.Fprintln(cmd.OutOrStdout())
					if err != nil {
						return err
					}
					token = strings.TrimSpace(string(b))
				} else {
					line, err := in.ReadString('\n')
					if err != nil {
						return err
					}
					token = strings.TrimSpace(line)
				}
			}
			if token == "" {
				return fmt.Errorf("token is required")
			}

			client, err := api.New(url, token)
			if err != nil {
				return err
			}
			client.Verbose = flagVerbose
			me, err := client.Me(cmd.Context())
			if err != nil {
				return fmt.Errorf("token check against %s failed: %w", url, err)
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			name := nameFlag
			if name == "" {
				name = "default"
			}
			cfg.Instances[name] = config.Instance{URL: url, Token: token}
			if cfg.Default == "" {
				cfg.Default = name
			}
			if err := config.Save(cfg); err != nil {
				return err
			}

			p, err := config.Path()
			if err != nil {
				p = "(unknown path)"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in to %s as %s (%s)\nConfig written to %s (instance %q)\n",
				url, me.DisplayName(), me.Email, p, name)
			return nil
		},
	}
	cmd.Flags().StringVar(&urlFlag, "url", "", "instance URL (prompted if omitted)")
	cmd.Flags().StringVar(&tokenFlag, "token", "", "API access token (prompted if omitted)")
	cmd.Flags().StringVar(&nameFlag, "name", "", `instance name in the config (default "default")`)
	return cmd
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active instance and authenticated user",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, url, _, err := config.Resolve(flagInstance)
			if err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			me, err := client.Me(cmd.Context())
			if err != nil {
				return fmt.Errorf("instance %s (%s): token check failed: %w", name, url, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Instance: %s (%s)\nUser:     %s (%s)\n", name, url, me.DisplayName(), me.Email)
			return nil
		},
	}
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the active instance from the config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			name := flagInstance
			if name == "" {
				name = os.Getenv(config.EnvInstance)
			}
			if name == "" {
				name = cfg.Default
			}
			if name == "" {
				return fmt.Errorf("no instance to log out from")
			}
			if _, ok := cfg.Instances[name]; !ok {
				return fmt.Errorf("unknown instance %q", name)
			}
			delete(cfg.Instances, name)
			if cfg.Default == name {
				cfg.Default = ""
				for k := range cfg.Instances {
					cfg.Default = k
					break
				}
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed instance %q from config\n", name)
			return nil
		},
	}
}
