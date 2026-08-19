package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/lukeisontheroad/zammad-cli/internal/output"
	"github.com/spf13/cobra"
)

// newAPICmd is the raw API escape hatch.
func newAPICmd() *cobra.Command {
	var fields []string
	cmd := &cobra.Command{
		Use:   "api <method> <path>",
		Short: "Make a raw Zammad API request",
		Long: `Make an authenticated request against any Zammad API endpoint and print the
JSON response. For GET/DELETE, --field values become query parameters; for
other methods they form the JSON request body (string values).`,
		Example: `  zammad api GET /api/v1/ticket_states
  zammad api GET /api/v1/tickets/search --field query=tags:billing --field expand=true
  zammad api PUT /api/v1/tickets/42 --field state=closed`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := strings.ToUpper(args[0])
			path := args[1]
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}

			kv := url.Values{}
			for _, f := range fields {
				k, v, ok := strings.Cut(f, "=")
				if !ok {
					return fmt.Errorf("invalid --field %q (want key=value)", f)
				}
				kv.Add(k, v)
			}

			var query url.Values
			var body any
			if method == http.MethodGet || method == http.MethodDelete {
				if len(kv) > 0 {
					query = kv
				}
			} else if len(kv) > 0 {
				// JSON bodies take the last value per key; repeated keys only
				// make sense as query parameters.
				m := map[string]string{}
				for k, vs := range kv {
					m[k] = vs[len(vs)-1]
				}
				body = m
			}

			client, err := newClient()
			if err != nil {
				return err
			}
			var raw json.RawMessage
			if err := client.Do(cmd.Context(), method, path, query, body, &raw); err != nil {
				return err
			}
			if len(raw) == 0 {
				return nil
			}
			return output.JSON(cmd.OutOrStdout(), raw)
		},
	}
	cmd.Flags().StringArrayVarP(&fields, "field", "f", nil, "key=value pair (query param for GET/DELETE, JSON body otherwise)")
	return cmd
}
