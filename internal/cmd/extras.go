package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lukeisontheroad/zammad-cli/internal/api"
	"github.com/lukeisontheroad/zammad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newTicketAttachmentsCmd() *cobra.Command {
	var download bool
	var dir string
	cmd := &cobra.Command{
		Use:   "attachments <id>",
		Short: "List or download a ticket's attachments",
		Example: `  zammad ticket attachments 42
  zammad ticket attachments 42 --download --dir ./logs`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(); err != nil {
				return err
			}
			id, err := parseTicketID(args[0])
			if err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			articles, err := client.ListArticles(cmd.Context(), id)
			if err != nil {
				return err
			}

			type att struct {
				articleID, attachmentID int
				filename, size          string
			}
			var atts []att
			for _, a := range articles {
				for _, x := range a.Attachments {
					atts = append(atts, att{a.ID, x.ID, x.Filename, x.Size})
				}
			}
			if len(atts) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No attachments.")
				return nil
			}

			if !download {
				rows := make([][]string, len(atts))
				for i, x := range atts {
					rows[i] = []string{strconv.Itoa(x.articleID), strconv.Itoa(x.attachmentID), x.filename, x.size}
				}
				output.Table(cmd.OutOrStdout(), []string{"article", "attachment", "filename", "size"}, rows)
				return nil
			}

			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			for _, x := range atts {
				data, err := client.DownloadAttachment(cmd.Context(), id, x.articleID, x.attachmentID)
				if err != nil {
					return fmt.Errorf("download %s: %w", x.filename, err)
				}
				name := filepath.Base(x.filename)
				path := filepath.Join(dir, name)
				if _, err := os.Stat(path); err == nil {
					path = filepath.Join(dir, fmt.Sprintf("%d_%s", x.attachmentID, name))
				}
				if err := os.WriteFile(path, data, 0o644); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Saved %s (%d bytes)\n", path, len(data))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&download, "download", false, "download all attachments")
	cmd.Flags().StringVar(&dir, "dir", ".", "download target directory")
	return cmd
}

func newTicketTagCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "List, add, or remove ticket tags",
	}
	list := &cobra.Command{
		Use:   "list <id>",
		Short: "List a ticket's tags",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseTicketID(args[0])
			if err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			tags, err := client.TicketTags(cmd.Context(), id)
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				return output.JSON(cmd.OutOrStdout(), tags)
			}
			if len(tags) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No tags.")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), strings.Join(tags, "\n"))
			return nil
		},
	}
	add := &cobra.Command{
		Use:   "add <id> <tag>...",
		Short: "Add tags to a ticket",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseTicketID(args[0])
			if err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			for _, tag := range args[1:] {
				if err := client.AddTicketTag(cmd.Context(), id, tag); err != nil {
					return fmt.Errorf("add tag %q: %w", tag, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Added tag %q to ticket #%d\n", tag, id)
			}
			return nil
		},
	}
	remove := &cobra.Command{
		Use:   "remove <id> <tag>...",
		Short: "Remove tags from a ticket",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseTicketID(args[0])
			if err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			for _, tag := range args[1:] {
				if err := client.RemoveTicketTag(cmd.Context(), id, tag); err != nil {
					return fmt.Errorf("remove tag %q: %w", tag, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Removed tag %q from ticket #%d\n", tag, id)
			}
			return nil
		},
	}
	cmd.AddCommand(list, add, remove)
	return cmd
}

func newTicketHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history <id>",
		Short: "Show a ticket's change history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(); err != nil {
				return err
			}
			id, err := parseTicketID(args[0])
			if err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			entries, users, err := client.TicketHistory(cmd.Context(), id)
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				return output.JSON(cmd.OutOrStdout(), entries)
			}
			var rows [][]string
			for _, e := range entries {
				what := e.Object + " " + e.Type
				change := ""
				if e.Attribute != "" {
					what = e.Object + " " + e.Attribute
					change = fmt.Sprintf("%v -> %v", orDash(e.ValueFrom), orDash(e.ValueTo))
				}
				by := users[e.CreatedByID]
				if by == "" {
					by = strconv.Itoa(e.CreatedByID)
				}
				rows = append(rows, []string{shortDate(e.CreatedAt), by, what, change})
			}
			output.Table(cmd.OutOrStdout(), []string{"when", "by", "what", "change"}, rows)
			return nil
		},
	}
}

func orDash(v any) string {
	if v == nil {
		return "-"
	}
	s := fmt.Sprintf("%v", v)
	if s == "" {
		return "-"
	}
	return s
}

func newOverviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "overview",
		Short: "Work with saved ticket overviews",
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List your overviews with ticket counts",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(); err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			overviews, err := client.ListOverviews(cmd.Context())
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				return output.JSON(cmd.OutOrStdout(), overviews)
			}
			rows := make([][]string, len(overviews))
			for i, o := range overviews {
				rows[i] = []string{o.Link, o.Name, strconv.Itoa(o.Count)}
			}
			output.Table(cmd.OutOrStdout(), []string{"link", "name", "tickets"}, rows)
			return nil
		},
	}
	var limit int
	view := &cobra.Command{
		Use:   "view <link>",
		Short: "List the tickets in one overview",
		Example: `  zammad overview view my_assigned
  zammad overview view open_unassigned`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(); err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			ids, err := client.OverviewTicketIDs(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if len(ids) > limit {
				fmt.Fprintf(cmd.ErrOrStderr(), "Showing %d of %d tickets (raise with --limit)\n", limit, len(ids))
				ids = ids[:limit]
			}
			tickets := make([]api.Ticket, 0, len(ids))
			for _, id := range ids {
				t, err := client.GetTicket(cmd.Context(), id)
				if err != nil {
					return err
				}
				tickets = append(tickets, *t)
			}
			return renderTickets(cmd, client, tickets)
		},
	}
	view.Flags().IntVar(&limit, "limit", 30, "maximum number of tickets")
	cmd.AddCommand(list, view)
	return cmd
}

func newSearchCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "search <term>",
		Short: "Search across tickets, users, and organizations",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(); err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			results, err := client.GlobalSearch(cmd.Context(), strings.Join(args, " "), limit)
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				return output.JSON(cmd.OutOrStdout(), results)
			}
			rows := make([][]string, len(results))
			for i, r := range results {
				rows[i] = []string{r.Type, strconv.Itoa(r.ID), output.Truncate(r.Label, 60), r.Extra}
			}
			output.Table(cmd.OutOrStdout(), []string{"type", "id", "name", "info"}, rows)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 30, "maximum number of results")
	return cmd
}

func newUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Look up users",
	}
	var limit int
	search := &cobra.Command{
		Use:   "search <term>",
		Short: "Search users by name, email, or login",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(); err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			users, err := client.SearchUsers(cmd.Context(), strings.Join(args, " "), limit)
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				return output.JSON(cmd.OutOrStdout(), users)
			}
			rows := make([][]string, len(users))
			for i, u := range users {
				rows[i] = []string{strconv.Itoa(u.ID), u.DisplayName(), u.Email, u.Login}
			}
			output.Table(cmd.OutOrStdout(), []string{"id", "name", "email", "login"}, rows)
			return nil
		},
	}
	search.Flags().IntVar(&limit, "limit", 30, "maximum number of users")
	cmd.AddCommand(search)
	return cmd
}

func newOrgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "Look up organizations",
	}
	var limit int
	search := &cobra.Command{
		Use:   "search <term>",
		Short: "Search organizations by name",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(); err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			orgs, err := client.SearchOrganizations(cmd.Context(), strings.Join(args, " "), limit)
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				return output.JSON(cmd.OutOrStdout(), orgs)
			}
			rows := make([][]string, len(orgs))
			for i, o := range orgs {
				rows[i] = []string{strconv.Itoa(o.ID), o.Name, o.Domain}
			}
			output.Table(cmd.OutOrStdout(), []string{"id", "name", "domain"}, rows)
			return nil
		},
	}
	search.Flags().IntVar(&limit, "limit", 30, "maximum number of organizations")
	cmd.AddCommand(search)
	return cmd
}
