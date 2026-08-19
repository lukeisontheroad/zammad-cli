package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/lukeisontheroad/zammad-cli/internal/api"
	"github.com/lukeisontheroad/zammad-cli/internal/output"
	"github.com/spf13/cobra"
)

func newTicketCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ticket",
		Short: "List, view, search, and modify tickets",
	}
	cmd.AddCommand(
		newTicketListCmd(),
		newTicketViewCmd(),
		newTicketSearchCmd(),
		newTicketCreateCmd(),
		newTicketUpdateCmd(),
		newTicketCloseCmd(),
		newTicketReplyCmd(),
		newTicketAttachmentsCmd(),
		newTicketTagCmd(),
		newTicketHistoryCmd(),
	)
	return cmd
}

// parseTicketID accepts a plain id ("58013", "#58013") or a ticket zoom URL
// ("https://host/#ticket/zoom/58013").
func parseTicketID(arg string) (int, error) {
	s := arg
	if _, rest, ok := strings.Cut(s, "#ticket/zoom/"); ok {
		s = rest
		if i := strings.IndexAny(s, "/?"); i >= 0 {
			s = s[:i]
		}
	}
	id, err := strconv.Atoi(strings.TrimPrefix(s, "#"))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid ticket id %q", arg)
	}
	return id, nil
}

// quoteQueryValue quotes a value for the Zammad (Elasticsearch) query syntax.
func quoteQueryValue(v string) string {
	return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
}

var emailTermRe = regexp.MustCompile(`^[a-z0-9.@_+-]+$`)

// customerQuery builds a fuzzy customer/company clause. Many Zammad setups
// never create organization records, so a company name must also match
// customer email domains and plain mentions in ticket text.
func customerQuery(term string) string {
	t := strings.ToLower(strings.TrimSpace(term))
	var parts []string
	if strings.Contains(t, "@") && emailTermRe.MatchString(t) {
		// Full address: exact customer match.
		parts = append(parts, "customer.email:"+quoteQueryValue(t))
	} else if emailTermRe.MatchString(t) {
		// Single word ("acme", "acme.com"): match anywhere in customer emails.
		// Wildcards only work unquoted.
		parts = append(parts, "customer.email:*"+t+"*")
	}
	parts = append(parts,
		"organization.name:"+quoteQueryValue(term),
		quoteQueryValue(term), // fulltext: subject/body mentions
	)
	return "(" + strings.Join(parts, " OR ") + ")"
}

// ownerNames resolves ticket owner ids to display names. Zammad instances may
// use opaque logins (e.g. UUIDs), so the expanded "owner" string is not always
// readable. Failures fall back to the expanded login per ticket.
func ownerNames(cmd *cobra.Command, client *api.Client, tickets []api.Ticket) map[int]string {
	names := map[int]string{}
	for _, t := range tickets {
		// owner_id 1 is Zammad's built-in "-" (nobody).
		if t.OwnerID <= 1 {
			continue
		}
		if _, done := names[t.OwnerID]; done {
			continue
		}
		if u, err := client.GetUser(cmd.Context(), t.OwnerID); err == nil {
			name := u.DisplayName()
			if name == "" {
				name = u.Email
			}
			names[t.OwnerID] = name
		}
	}
	return names
}

func ownerDisplay(names map[int]string, t api.Ticket) string {
	if n, ok := names[t.OwnerID]; ok && n != "" {
		return n
	}
	return t.Owner
}

func renderTickets(cmd *cobra.Command, client *api.Client, tickets []api.Ticket) error {
	if flagOutput == "json" {
		raws := make([]json.RawMessage, len(tickets))
		for i, t := range tickets {
			raws[i] = t.Raw
		}
		return output.JSON(cmd.OutOrStdout(), raws)
	}
	names := ownerNames(cmd, client, tickets)
	rows := make([][]string, len(tickets))
	for i, t := range tickets {
		rows[i] = []string{
			strconv.Itoa(t.ID),
			t.State,
			t.Priority,
			output.Truncate(t.Title, 60),
			t.Group,
			ownerDisplay(names, t),
			shortDate(t.UpdatedAt),
		}
	}
	output.Table(cmd.OutOrStdout(), []string{"id", "state", "priority", "title", "group", "owner", "updated"}, rows)
	return nil
}

func shortDate(iso string) string {
	if len(iso) >= 16 {
		return strings.Replace(iso[:16], "T", " ", 1)
	}
	return iso
}

func newTicketListCmd() *cobra.Command {
	var state, group, owner, customer string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tickets (open by default)",
		Example: `  zammad ticket list
  zammad ticket list --state all --limit 100
  zammad ticket list --owner me --state open
  zammad ticket list --group Support -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(); err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}

			var parts []string
			if state != "" && state != "all" {
				parts = append(parts, "state.name:"+quoteQueryValue(state))
			}
			if group != "" {
				parts = append(parts, "group.name:"+quoteQueryValue(group))
			}
			if owner != "" {
				login := owner
				if owner == "me" {
					me, err := client.Me(cmd.Context())
					if err != nil {
						return err
					}
					login = me.Login
				}
				parts = append(parts, "owner.login:"+quoteQueryValue(login))
			}
			if customer != "" {
				parts = append(parts, customerQuery(customer))
			}

			var tickets []api.Ticket
			if len(parts) == 0 {
				tickets, err = client.ListTickets(cmd.Context(), limit)
			} else {
				tickets, err = client.SearchTickets(cmd.Context(), strings.Join(parts, " AND "), limit)
			}
			if err != nil {
				return err
			}
			return renderTickets(cmd, client, tickets)
		},
	}
	cmd.Flags().StringVar(&state, "state", "open", `filter by state name ("all" to disable)`)
	cmd.Flags().StringVar(&group, "group", "", "filter by group name")
	cmd.Flags().StringVar(&owner, "owner", "", `filter by owner login ("me" for yourself)`)
	cmd.Flags().StringVar(&customer, "customer", "", "fuzzy customer/company filter (org name, email, or domain)")
	cmd.Flags().IntVar(&limit, "limit", 30, "maximum number of tickets")
	return cmd
}

func newTicketSearchCmd() *cobra.Command {
	var limit int
	var customer string
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search tickets with the Zammad query syntax",
		Long: `Search tickets using Zammad's Elasticsearch query syntax, passed through
verbatim.

Useful query fields:
  state.name:open              priority.name:"3 high"     group.name:Support
  owner.login:jdoe             customer.email:*acme.com   organization.name:ACME
  tags:billing                 number:20001               title:printer
Operators: AND, OR, parentheses, quotes for values with spaces, * wildcards,
date ranges like created_at:[2026-01-01 TO 2026-02-01]. Bare terms search
full text. Reference: https://user-docs.zammad.org/en/latest/advanced/search.html

--customer <term> adds a fuzzy customer/company clause: it matches the
organization name, customer email addresses/domains, and plain mentions in
ticket text. Useful when no organization records exist in Zammad.`,
		Example: `  zammad ticket search "state.name:open AND priority.name:\"3 high\""
  zammad ticket search "tags:billing"
  zammad ticket search --customer ACME
  zammad ticket search --customer acme.com "state.name:open"`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(); err != nil {
				return err
			}
			var parts []string
			if len(args) > 0 {
				parts = append(parts, strings.Join(args, " "))
			}
			if customer != "" {
				parts = append(parts, customerQuery(customer))
			}
			if len(parts) == 0 {
				return fmt.Errorf("provide a query, --customer, or both")
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			tickets, err := client.SearchTickets(cmd.Context(), strings.Join(parts, " AND "), limit)
			if err != nil {
				return err
			}
			return renderTickets(cmd, client, tickets)
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "fuzzy customer/company filter (org name, email, or domain)")
	cmd.Flags().IntVar(&limit, "limit", 30, "maximum number of tickets")
	return cmd
}

func newTicketViewCmd() *cobra.Command {
	var comments, web bool
	cmd := &cobra.Command{
		Use:   "view <id>",
		Short: "Show a ticket, optionally with its articles",
		Example: `  zammad ticket view 42
  zammad ticket view 42 --comments
  zammad ticket view 42 --web`,
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

			if web {
				url := fmt.Sprintf("%s/#ticket/zoom/%d", client.BaseURL(), id)
				fmt.Fprintln(cmd.OutOrStdout(), "Opening", url)
				return openBrowser(url)
			}

			t, err := client.GetTicket(cmd.Context(), id)
			if err != nil {
				return err
			}

			if flagOutput == "json" {
				if !comments {
					return output.JSON(cmd.OutOrStdout(), t.Raw)
				}
				articles, err := client.ListArticles(cmd.Context(), id)
				if err != nil {
					return err
				}
				return output.JSON(cmd.OutOrStdout(), map[string]any{
					"ticket":   t.Raw,
					"articles": articles,
				})
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "#%d  %s\n", t.ID, t.Title)
			fmt.Fprintf(out, "number:   %s\n", t.Number)
			fmt.Fprintf(out, "state:    %s\n", t.State)
			fmt.Fprintf(out, "priority: %s\n", t.Priority)
			fmt.Fprintf(out, "group:    %s\n", t.Group)
			fmt.Fprintf(out, "owner:    %s\n", ownerDisplay(ownerNames(cmd, client, []api.Ticket{*t}), *t))
			fmt.Fprintf(out, "customer: %s\n", t.Customer)
			fmt.Fprintf(out, "created:  %s\n", shortDate(t.CreatedAt))
			fmt.Fprintf(out, "updated:  %s\n", shortDate(t.UpdatedAt))

			if comments {
				articles, err := client.ListArticles(cmd.Context(), id)
				if err != nil {
					return err
				}
				for _, a := range articles {
					fmt.Fprintf(out, "\n--- %s  %s  (%s", shortDate(a.CreatedAt), a.From, a.Type)
					if a.Internal {
						fmt.Fprint(out, ", internal")
					}
					fmt.Fprintln(out, ") ---")
					fmt.Fprintln(out, strings.TrimSpace(a.Body))
					for _, att := range a.Attachments {
						fmt.Fprintf(out, "[attachment: %s]\n", att.Filename)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&comments, "comments", "c", false, "include ticket articles")
	cmd.Flags().BoolVarP(&web, "web", "w", false, "open the ticket in the browser")
	return cmd
}

func newTicketCreateCmd() *cobra.Command {
	var title, group, customer, body, priority, state string
	var attach []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a ticket",
		Example: `  zammad ticket create --title "Printer on fire" --group Support \
      --customer jane@example.com --body "It is literally on fire."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(); err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			atts, err := loadAttachments(attach)
			if err != nil {
				return err
			}
			in := &api.TicketCreate{
				Title:    title,
				Group:    group,
				Customer: customer,
				Priority: priority,
				State:    state,
				Article:  &api.TicketArticle{Body: body, Type: "note", ContentType: "text/plain", Attachments: atts},
			}
			t, err := client.CreateTicket(cmd.Context(), in)
			// Unknown email: retry with the guess: prefix, which auto-creates
			// the customer. (Sending guess: for an existing customer fails on
			// some instances, so it is only a fallback.)
			if isUnknownCustomer(err) && strings.Contains(customer, "@") && !strings.HasPrefix(customer, "guess:") {
				in.Customer = "guess:" + customer
				t, err = client.CreateTicket(cmd.Context(), in)
			}
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				return output.JSON(cmd.OutOrStdout(), t.Raw)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created ticket #%d (number %s): %s\n", t.ID, t.Number, t.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "ticket title (required)")
	cmd.Flags().StringVar(&group, "group", "", "group name (required)")
	cmd.Flags().StringVar(&customer, "customer", "", "customer email or login (required)")
	cmd.Flags().StringVar(&body, "body", "", "first article body (required)")
	cmd.Flags().StringVar(&priority, "priority", "", `priority name (e.g. "3 high")`)
	cmd.Flags().StringVar(&state, "state", "", "state name (default: new)")
	cmd.Flags().StringArrayVar(&attach, "attach", nil, "attach a file (repeatable)")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("group")
	_ = cmd.MarkFlagRequired("customer")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func newTicketUpdateCmd() *cobra.Command {
	var title, group, state, priority, owner string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update ticket attributes",
		Example: `  zammad ticket update 42 --state "pending reminder"
  zammad ticket update 42 --priority "3 high" --owner agent@example.com`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(); err != nil {
				return err
			}
			id, err := parseTicketID(args[0])
			if err != nil {
				return err
			}
			in := &api.TicketUpdate{Title: title, Group: group, State: state, Priority: priority, Owner: owner}
			if *in == (api.TicketUpdate{}) {
				return fmt.Errorf("nothing to update: pass at least one of --title, --group, --state, --priority, --owner")
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			t, err := client.UpdateTicket(cmd.Context(), id, in)
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				return output.JSON(cmd.OutOrStdout(), t.Raw)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated ticket #%d: state=%s priority=%s owner=%s\n", t.ID, t.State, t.Priority, t.Owner)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&group, "group", "", "new group name")
	cmd.Flags().StringVar(&state, "state", "", "new state name")
	cmd.Flags().StringVar(&priority, "priority", "", "new priority name")
	cmd.Flags().StringVar(&owner, "owner", "", "new owner login/email")
	return cmd
}

func newTicketCloseCmd() *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:     "close <id>",
		Short:   "Close a ticket",
		Example: `  zammad ticket close 42 --note "Fixed by rebooting the printer."`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(); err != nil {
				return err
			}
			id, err := parseTicketID(args[0])
			if err != nil {
				return err
			}
			in := &api.TicketUpdate{State: "closed"}
			if note != "" {
				in.Article = &api.TicketArticle{Body: note, Type: "note", Internal: true, ContentType: "text/plain"}
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			t, err := client.UpdateTicket(cmd.Context(), id, in)
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				return output.JSON(cmd.OutOrStdout(), t.Raw)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Closed ticket #%d\n", t.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "add an internal note while closing")
	return cmd
}

func newTicketReplyCmd() *cobra.Command {
	var body, to, cc, subject, timeUnit string
	var attach []string
	var internal, email, noSignature, replyAll, markdown bool
	cmd := &cobra.Command{
		Use:   "reply <id>",
		Short: "Add a note to a ticket, or send an email reply",
		Long: `Add an article to a ticket.

Default: a note, visible to the customer in the portal (use --internal to
hide it). No email is sent.

--email sends a real outbound email reply instead: like the Zammad UI, the
recipient defaults to the sender of the customer's last email on the ticket
(falling back to the ticket customer), override with --to / --cc. The group's
email signature is appended automatically; disable with --no-signature.
Requires an email channel configured for the ticket's group in Zammad.

--reply-all additionally CCs everyone from the customer's last email (its
To and Cc recipients), excluding the instance's own inbox addresses and you.

--markdown renders the body from Markdown (bold, lists, links, code, ...)
instead of treating it as plain text.

--attach adds files (repeatable). --time books accounted time units on the
article (required by instances that enforce time accounting).`,
		Example: `  zammad ticket reply 42 --body "Working on it."
  zammad ticket reply 42 --body "Vendor escalation filed." --internal
  zammad ticket reply 42 --body "Fix is deployed, please verify." --email
  zammad ticket reply 42 --body "See below." --email --to jane@example.com --cc boss@example.com`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(); err != nil {
				return err
			}
			id, err := parseTicketID(args[0])
			if err != nil {
				return err
			}
			if internal && email {
				return fmt.Errorf("--internal and --email are mutually exclusive")
			}
			if (to != "" || cc != "" || replyAll) && !email {
				return fmt.Errorf("--to/--cc/--reply-all require --email")
			}
			if replyAll && (to != "" || cc != "") {
				return fmt.Errorf("--reply-all and --to/--cc are mutually exclusive")
			}
			client, err := newClient()
			if err != nil {
				return err
			}

			article := &api.TicketArticle{Body: body, Type: "note", Internal: internal, ContentType: "text/plain", TimeUnit: timeUnit}
			article.Attachments, err = loadAttachments(attach)
			if err != nil {
				return err
			}
			bodyHTML := ""
			if markdown {
				bodyHTML, err = renderMarkdown(body)
				if err != nil {
					return err
				}
				article.Body = bodyHTML
				article.ContentType = "text/html"
			}
			if email {
				t, err := client.GetTicket(cmd.Context(), id)
				if err != nil {
					return err
				}
				article.Type = "email"
				article.To = to
				article.Cc = cc
				// Subject stays optional: the server falls back to the ticket
				// title and applies the "[Ticket#...]" hook itself per the
				// instance's ticket_hook_position setting.
				article.Subject = subject
				if article.To == "" {
					// Like the UI: answer the customer's last email article;
					// fall back to the ticket's customer.
					articles, err := client.ListArticles(cmd.Context(), id)
					if err != nil {
						return err
					}
					last := lastCustomerEmail(articles)
					if last != nil {
						if addr := extractAddr(last.ReplyTo); addr != "" {
							article.To = addr
						} else {
							article.To = extractAddr(last.From)
						}
					}
					if article.To == "" && strings.Contains(t.Customer, "@") {
						article.To = t.Customer
					}
					if article.To == "" {
						return fmt.Errorf("cannot determine recipient for ticket #%d, pass --to explicitly", id)
					}
					if replyAll && last != nil {
						article.Cc = replyAllCc(cmd, client, last, article.To)
					}
				}
				// Zammad only renders signatures in its web UI, so append the
				// group's signature here. Best effort: missing signature or
				// permission problems fall back to the bare body.
				article.Body = emailBody(cmd, client, t.GroupID, body, bodyHTML, noSignature)
				article.ContentType = "text/html"
			}

			t, err := client.UpdateTicket(cmd.Context(), id, &api.TicketUpdate{Article: article})
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				return output.JSON(cmd.OutOrStdout(), t.Raw)
			}
			switch {
			case email:
				fmt.Fprintf(cmd.OutOrStdout(), "Sent email reply on ticket #%d to %s\n", t.ID, article.To)
			case internal:
				fmt.Fprintf(cmd.OutOrStdout(), "Added internal note to ticket #%d\n", t.ID)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "Added note to ticket #%d\n", t.ID)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "article body (required)")
	cmd.Flags().BoolVar(&internal, "internal", false, "make the note internal (hidden from customer)")
	cmd.Flags().BoolVar(&email, "email", false, "send as outbound email instead of a note")
	cmd.Flags().StringVar(&to, "to", "", "email recipient (default: ticket customer; requires --email)")
	cmd.Flags().StringVar(&cc, "cc", "", "email CC (requires --email)")
	cmd.Flags().StringVar(&subject, "subject", "", "email subject (default: derived from ticket title)")
	cmd.Flags().BoolVar(&noSignature, "no-signature", false, "do not append the group's email signature (requires --email)")
	cmd.Flags().StringArrayVar(&attach, "attach", nil, "attach a file (repeatable)")
	cmd.Flags().StringVar(&timeUnit, "time", "", `accounted time units for this article (e.g. "15")`)
	cmd.Flags().BoolVar(&replyAll, "reply-all", false, "CC everyone from the customer's last email (requires --email)")
	cmd.Flags().BoolVar(&markdown, "markdown", false, "render the body from Markdown instead of plain text")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

// renderMarkdown converts Markdown to HTML for article bodies.
func renderMarkdown(src string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.New(goldmark.WithExtensions(extension.GFM)).
		Convert([]byte(src), &buf); err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// lastCustomerEmail returns the customer's most recent email article, or nil.
func lastCustomerEmail(articles []api.Article) *api.Article {
	for i := len(articles) - 1; i >= 0; i-- {
		if articles[i].Type == "email" && articles[i].Sender == "Customer" {
			return &articles[i]
		}
	}
	return nil
}

// replyAllCc mirrors the UI's "Reply all": everyone from the source email's
// To and Cc, minus the chosen To recipient, the instance's own inbox
// addresses, and the acting agent.
func replyAllCc(cmd *cobra.Command, client *api.Client, last *api.Article, to string) string {
	exclude := map[string]bool{strings.ToLower(to): true}
	if system, err := client.SystemEmailAddresses(cmd.Context()); err == nil {
		for _, a := range system {
			exclude[a] = true
		}
	} else if flagVerbose {
		fmt.Fprintf(cmd.ErrOrStderr(), "email address lookup failed, CC may include group inboxes: %v\n", err)
	}
	if me, err := client.Me(cmd.Context()); err == nil {
		exclude[strings.ToLower(me.Email)] = true
	}
	var cc []string
	seen := map[string]bool{}
	for _, list := range []string{last.To, last.Cc} {
		for _, addr := range parseAddressList(list) {
			key := strings.ToLower(addr)
			if exclude[key] || seen[key] {
				continue
			}
			seen[key] = true
			cc = append(cc, addr)
		}
	}
	return strings.Join(cc, ", ")
}

// parseAddressList extracts bare addresses from a header-style list like
// "Jane <j@x.com>, support@example.com".
func parseAddressList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	if parsed, err := mail.ParseAddressList(s); err == nil {
		addrs := make([]string, 0, len(parsed))
		for _, p := range parsed {
			addrs = append(addrs, p.Address)
		}
		return addrs
	}
	// Malformed header: salvage what looks like addresses.
	var addrs []string
	for _, part := range strings.Split(s, ",") {
		if addr := extractAddr(part); addr != "" {
			addrs = append(addrs, addr)
		}
	}
	return addrs
}

var emailAddrRe = regexp.MustCompile(`<([^>]+@[^>]+)>`)

// extractAddr pulls the bare address out of "Name <addr>" formats.
func extractAddr(s string) string {
	if m := emailAddrRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	s = strings.TrimSpace(s)
	if strings.Contains(s, "@") && !strings.ContainsAny(s, " ,;") {
		return s
	}
	return ""
}

// replyRecipient mirrors the UI's "Reply" target: the sender of the
// customer's most recent email article (Reply-To preferred over From).
func replyRecipient(articles []api.Article) string {
	a := lastCustomerEmail(articles)
	if a == nil {
		return ""
	}
	if addr := extractAddr(a.ReplyTo); addr != "" {
		return addr
	}
	return extractAddr(a.From)
}

// loadAttachments reads files and encodes them for the Zammad article payload.
func loadAttachments(paths []string) ([]api.ArticleAttachment, error) {
	var atts []api.ArticleAttachment
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("attach %s: %w", p, err)
		}
		mimeType := mime.TypeByExtension(filepath.Ext(p))
		if mimeType == "" {
			mimeType = http.DetectContentType(data)
		}
		atts = append(atts, api.ArticleAttachment{
			Filename: filepath.Base(p),
			Data:     base64.StdEncoding.EncodeToString(data),
			MimeType: mimeType,
		})
	}
	return atts, nil
}

var signaturePlaceholderRe = regexp.MustCompile(`#\{user\.([a-zA-Z_]+)\}`)

// emailBody converts the body to HTML (unless bodyHTML is already rendered,
// e.g. from Markdown) and appends the group's rendered signature.
func emailBody(cmd *cobra.Command, client *api.Client, groupID int, body, bodyHTML string, noSignature bool) string {
	htmlBody := bodyHTML
	if htmlBody == "" {
		htmlBody = strings.ReplaceAll(htmlEscape(body), "\n", "<br>")
	}
	if noSignature {
		return htmlBody
	}
	sig, err := client.GroupSignature(cmd.Context(), groupID)
	if err != nil || sig == nil {
		if err != nil && flagVerbose {
			fmt.Fprintf(cmd.ErrOrStderr(), "signature lookup failed, sending without: %v\n", err)
		}
		return htmlBody
	}
	me, err := client.MeRaw(cmd.Context())
	if err != nil {
		me = map[string]any{}
	}
	rendered := signaturePlaceholderRe.ReplaceAllStringFunc(sig.Body, func(m string) string {
		attr := signaturePlaceholderRe.FindStringSubmatch(m)[1]
		switch v := me[attr].(type) {
		case string:
			return v
		case float64:
			return fmt.Sprintf("%v", v)
		default:
			return ""
		}
	})
	// Same structure the Zammad UI produces: body, blank line, marked signature.
	return htmlBody + `<br><br><div data-signature="true" data-signature-id="` +
		strconv.Itoa(sig.ID) + `">` + rendered + `</div>`
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// isUnknownCustomer reports whether err is Zammad's 422 "No lookup value
// found for 'customer'" response.
func isUnknownCustomer(err error) bool {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == 422 && strings.Contains(apiErr.Message, "'customer'")
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
