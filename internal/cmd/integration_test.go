package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lukeisontheroad/zammad-cli/internal/api"
)

// mockZammad is a minimal fake Zammad API for command-level tests. It records
// requests so tests can assert on queries and payloads.
type mockZammad struct {
	srv      *httptest.Server
	queries  []string          // ticket search "query" params, in order
	posts    []string          // ticket create bodies, in order
	payloads map[string]string // "METHOD path" -> last request body
}

func newMockZammad(t *testing.T) *mockZammad {
	t.Helper()
	m := &mockZammad{payloads: map[string]string{}}
	mux := http.NewServeMux()

	const ticket7 = `{"id":7,"number":"20001","title":"Printer on fire","state":"open","priority":"3 high","group":"Support","group_id":38,"owner":"uuid-login-1","owner_id":9,"customer":"jane@example.com","created_at":"2026-08-18T09:00:00Z","updated_at":"2026-08-19T08:30:00Z"}`

	mux.HandleFunc("/api/v1/users/me", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"agent","firstname":"Ada","lastname":"Lovelace","email":"ada@example.com"}`)
	})
	mux.HandleFunc("/api/v1/users/9", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":9,"login":"uuid-login-1","firstname":"Grace","lastname":"Hopper","email":"grace@example.com"}`)
	})
	mux.HandleFunc("/api/v1/users/search", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":9,"login":"grace","firstname":"Grace","lastname":"Hopper","email":"grace@example.com"}]`)
	})
	mux.HandleFunc("/api/v1/organizations/search", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":3,"name":"ACME Corp","domain":"acme.com","active":true}]`)
	})
	mux.HandleFunc("/api/v1/tickets/search", func(w http.ResponseWriter, r *http.Request) {
		m.queries = append(m.queries, r.URL.Query().Get("query"))
		fmt.Fprintf(w, "[%s]", ticket7)
	})
	mux.HandleFunc("/api/v1/tickets/7", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			m.payloads["PUT /api/v1/tickets/7"] = string(body)
			fmt.Fprint(w, `{"id":7,"number":"20001","title":"Printer on fire","state":"closed","priority":"3 high","owner":"agent","owner_id":9}`)
			return
		}
		fmt.Fprint(w, ticket7)
	})
	mux.HandleFunc("/api/v1/tickets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			raw, _ := io.ReadAll(r.Body)
			body := string(raw)
			m.posts = append(m.posts, body)
			m.payloads["POST /api/v1/tickets"] = body
			// Unknown customers require the guess: prefix, mirroring Zammad.
			if strings.Contains(body, `"customer":"unknown@example.com"`) {
				w.WriteHeader(http.StatusUnprocessableEntity)
				fmt.Fprint(w, `{"error":"No lookup value found for 'customer': \"unknown@example.com\""}`)
				return
			}
			fmt.Fprint(w, `{"id":8,"number":"20002","title":"New ticket","state":"new"}`)
			return
		}
		fmt.Fprintf(w, "[%s]", ticket7)
	})
	mux.HandleFunc("/api/v1/ticket_articles/by_ticket/7", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":11,"ticket_id":7,"type":"email","sender":"Customer","from":"Jane Doe <jane.alias@example.com>","to":"support@example.com, Colleague <colleague@example.com>","cc":"watcher@example.com, ada@example.com","body":"It is on fire.","internal":false,"created_at":"2026-08-18T09:00:00Z","attachments":[{"id":5,"filename":"fire.log","size":"10"}]},{"id":12,"ticket_id":7,"type":"note","sender":"Agent","from":"Agent Smith","body":"internal","internal":true,"created_at":"2026-08-18T10:00:00Z"}]`)
	})
	mux.HandleFunc("/api/v1/ticket_attachment/7/11/5", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "burning logs")
	})
	mux.HandleFunc("/api/v1/tags", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tags":["billing","urgent"]}`)
	})
	mux.HandleFunc("/api/v1/tags/add", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		m.payloads["POST /api/v1/tags/add"] = string(body)
		fmt.Fprint(w, `{}`)
	})
	mux.HandleFunc("/api/v1/tags/remove", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		m.payloads["DELETE /api/v1/tags/remove"] = string(body)
		fmt.Fprint(w, `{}`)
	})
	mux.HandleFunc("/api/v1/ticket_history/7", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"history":[{"object":"Ticket","type":"updated","attribute":"state","value_from":"new","value_to":"open","created_by_id":9,"created_at":"2026-08-18T10:00:00Z"}],"assets":{"User":{"9":{"id":9,"firstname":"Grace","lastname":"Hopper"}}}}`)
	})
	mux.HandleFunc("/api/v1/ticket_overviews", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("view") != "" {
			fmt.Fprint(w, `{"index":{"tickets":[{"id":7}]}}`)
			return
		}
		fmt.Fprint(w, `[{"id":1,"name":"My open Tickets","link":"my_assigned","count":1}]`)
	})
	mux.HandleFunc("/api/v1/search", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":[{"type":"Ticket","id":7},{"type":"User","id":9}],"assets":{"Ticket":{"7":{"id":7,"title":"Printer on fire","state_id":2}},"User":{"9":{"id":9,"firstname":"Grace","lastname":"Hopper","email":"grace@example.com"}}}}`)
	})
	mux.HandleFunc("/api/v1/ticket_states", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":2,"name":"open"}]`)
	})
	mux.HandleFunc("/api/v1/email_addresses", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":1,"email":"support@example.com"}]`)
	})
	mux.HandleFunc("/api/v1/signatures", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":4,"name":"support","body":"Regards,<br>#{user.firstname} #{user.lastname}","active":true,"group_ids":[38]},{"id":5,"name":"inactive","body":"OLD","active":false,"group_ids":[38]}]`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error":"no route: %s %s"}`, r.Method, r.URL.Path)
	})

	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

// run executes the CLI against the mock and returns stdout.
func (m *mockZammad) run(t *testing.T, args ...string) string {
	t.Helper()
	out, err := m.runErr(t, args...)
	if err != nil {
		t.Fatalf("zammad %s: %v", strings.Join(args, " "), err)
	}
	return out
}

func (m *mockZammad) runErr(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Setenv("ZAMMAD_URL", m.srv.URL)
	t.Setenv("ZAMMAD_TOKEN", "testtoken")
	t.Setenv("ZAMMAD_CONFIG", filepath.Join(t.TempDir(), "config.yml"))
	root := newRootCmd("test")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestTicketListTable(t *testing.T) {
	m := newMockZammad(t)
	out := m.run(t, "ticket", "list")
	for _, want := range []string{"Printer on fire", "3 high", "Grace Hopper", "Support"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "uuid-login-1") {
		t.Errorf("owner login not resolved to name:\n%s", out)
	}
	if got := m.queries[0]; !strings.Contains(got, `state.name:"open"`) {
		t.Errorf("default state filter missing, query: %s", got)
	}
}

func TestTicketListJSON(t *testing.T) {
	m := newMockZammad(t)
	out := m.run(t, "ticket", "list", "-o", "json")
	var tickets []map[string]any
	if err := json.Unmarshal([]byte(out), &tickets); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(tickets) != 1 || tickets[0]["number"] != "20001" {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestTicketSearchCustomerQuery(t *testing.T) {
	m := newMockZammad(t)
	m.run(t, "ticket", "search", "--customer", "ACME")
	q := m.queries[0]
	for _, want := range []string{"customer.email:*acme*", `organization.name:"ACME"`, `"ACME"`, " OR "} {
		if !strings.Contains(q, want) {
			t.Errorf("customer query missing %q: %s", want, q)
		}
	}
}

func TestTicketViewWithComments(t *testing.T) {
	m := newMockZammad(t)
	out := m.run(t, "ticket", "view", "7", "--comments")
	for _, want := range []string{"#7  Printer on fire", "jane@example.com", "It is on fire.", "[attachment: fire.log]"} {
		if !strings.Contains(out, want) {
			t.Errorf("view output missing %q:\n%s", want, out)
		}
	}
}

func TestTicketCreateExistingCustomer(t *testing.T) {
	m := newMockZammad(t)
	out := m.run(t, "ticket", "create", "--title", "New ticket", "--group", "Support",
		"--customer", "jane@example.com", "--body", "hi")
	if !strings.Contains(out, "Created ticket #8") {
		t.Errorf("unexpected output: %s", out)
	}
	// Known customer: plain email, no guess: prefix, single request.
	if len(m.posts) != 1 || !strings.Contains(m.posts[0], `"customer":"jane@example.com"`) {
		t.Errorf("expected one plain-email create, got %v", m.posts)
	}
}

func TestTicketCreateGuessFallback(t *testing.T) {
	m := newMockZammad(t)
	out := m.run(t, "ticket", "create", "--title", "New ticket", "--group", "Support",
		"--customer", "unknown@example.com", "--body", "hi")
	if !strings.Contains(out, "Created ticket #8") {
		t.Errorf("unexpected output: %s", out)
	}
	// Unknown customer: plain attempt 422s, retry carries the guess: prefix.
	if len(m.posts) != 2 ||
		!strings.Contains(m.posts[0], `"customer":"unknown@example.com"`) ||
		!strings.Contains(m.posts[1], `"customer":"guess:unknown@example.com"`) {
		t.Errorf("expected plain then guess: retry, got %v", m.posts)
	}
}

func TestTicketReplyInternal(t *testing.T) {
	m := newMockZammad(t)
	m.run(t, "ticket", "reply", "7", "--body", "on it", "--internal")
	payload := m.payloads["PUT /api/v1/tickets/7"]
	if !strings.Contains(payload, `"internal":true`) || !strings.Contains(payload, `"body":"on it"`) {
		t.Errorf("reply payload wrong: %s", payload)
	}
}

func TestTicketReplyEmailDefaultsToLastCustomerEmail(t *testing.T) {
	m := newMockZammad(t)
	out := m.run(t, "ticket", "reply", "7", "--body", "fix deployed", "--email")
	// Recipient comes from the customer's last email article (alias address),
	// not the plain ticket customer.
	if !strings.Contains(out, "to jane.alias@example.com") {
		t.Errorf("unexpected output: %s", out)
	}
	a := decodeArticle(t, m.payloads["PUT /api/v1/tickets/7"])
	if a["type"] != "email" || a["to"] != "jane.alias@example.com" || a["internal"] != false || a["content_type"] != "text/html" {
		t.Errorf("email article fields wrong: %v", a)
	}
	// Subject left to the server, which applies title fallback + ticket hook.
	if _, ok := a["subject"]; ok {
		t.Errorf("subject should be omitted when not passed: %v", a["subject"])
	}
	body, _ := a["body"].(string)
	// Signature appended with rendered placeholders.
	if !strings.Contains(body, "Regards,<br>Ada Lovelace") || !strings.Contains(body, `data-signature="true"`) {
		t.Errorf("signature missing/unrendered in body: %s", body)
	}
}

func TestTicketReplyEmailNoSignature(t *testing.T) {
	m := newMockZammad(t)
	m.run(t, "ticket", "reply", "7", "--body", "line1\nline2 <x>", "--email", "--no-signature")
	body, _ := decodeArticle(t, m.payloads["PUT /api/v1/tickets/7"])["body"].(string)
	if strings.Contains(body, "data-signature") {
		t.Errorf("signature appended despite --no-signature: %s", body)
	}
	// Plain text became escaped HTML with <br> line breaks.
	if body != "line1<br>line2 &lt;x&gt;" {
		t.Errorf("body not HTML-converted: %s", body)
	}
}

func TestTicketReplyAllCc(t *testing.T) {
	m := newMockZammad(t)
	m.run(t, "ticket", "reply", "7", "--body", "answer", "--email", "--reply-all")
	a := decodeArticle(t, m.payloads["PUT /api/v1/tickets/7"])
	if a["to"] != "jane.alias@example.com" {
		t.Errorf("to wrong: %v", a["to"])
	}
	// CC keeps other humans; drops the group inbox (support@), the acting
	// agent (ada@), and the To recipient.
	if a["cc"] != "colleague@example.com, watcher@example.com" {
		t.Errorf("cc wrong: %v", a["cc"])
	}
}

func TestTicketReplyMarkdown(t *testing.T) {
	m := newMockZammad(t)
	m.run(t, "ticket", "reply", "7", "--body", "**bold** and `code`", "--email", "--markdown", "--no-signature")
	a := decodeArticle(t, m.payloads["PUT /api/v1/tickets/7"])
	body, _ := a["body"].(string)
	if !strings.Contains(body, "<strong>bold</strong>") || !strings.Contains(body, "<code>code</code>") {
		t.Errorf("markdown not rendered: %s", body)
	}
	// Markdown notes (no --email) become HTML articles too.
	m.run(t, "ticket", "reply", "7", "--body", "* item", "--markdown")
	a = decodeArticle(t, m.payloads["PUT /api/v1/tickets/7"])
	if a["content_type"] != "text/html" || !strings.Contains(a["body"].(string), "<li>item</li>") {
		t.Errorf("markdown note wrong: %v", a)
	}
}

func TestTicketReplyAttachAndTime(t *testing.T) {
	m := newMockZammad(t)
	f := filepath.Join(t.TempDir(), "log.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.run(t, "ticket", "reply", "7", "--body", "see log", "--attach", f, "--time", "15")
	a := decodeArticle(t, m.payloads["PUT /api/v1/tickets/7"])
	if a["time_unit"] != "15" {
		t.Errorf("time_unit missing: %v", a)
	}
	atts, _ := a["attachments"].([]any)
	if len(atts) != 1 {
		t.Fatalf("expected one attachment: %v", a)
	}
	att := atts[0].(map[string]any)
	if att["filename"] != "log.txt" || att["data"] != "aGVsbG8=" || !strings.HasPrefix(att["mime-type"].(string), "text/plain") {
		t.Errorf("attachment payload wrong: %v", att)
	}
}

func TestReplyRecipientHelpers(t *testing.T) {
	if got := extractAddr("Jane <j@x.com>"); got != "j@x.com" {
		t.Errorf("extractAddr angle form: %s", got)
	}
	if got := extractAddr("j@x.com"); got != "j@x.com" {
		t.Errorf("extractAddr bare form: %s", got)
	}
	if got := extractAddr("no address here"); got != "" {
		t.Errorf("extractAddr junk: %s", got)
	}
	// Reply-To wins over From; agents and notes are skipped.
	articles := []api.Article{
		{Type: "email", Sender: "Customer", From: "old@x.com"},
		{Type: "email", Sender: "Customer", From: "Jane <j@x.com>", ReplyTo: "billing@x.com"},
		{Type: "email", Sender: "Agent", From: "agent@x.com"},
		{Type: "note", Sender: "Customer", From: "ignored@x.com"},
	}
	if got := replyRecipient(articles); got != "billing@x.com" {
		t.Errorf("replyRecipient: %s", got)
	}
}

func decodeArticle(t *testing.T, payload string) map[string]any {
	t.Helper()
	var p struct {
		Article map[string]any `json:"article"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, payload)
	}
	return p.Article
}

func TestTicketReplyEmailInternalConflict(t *testing.T) {
	m := newMockZammad(t)
	if _, err := m.runErr(t, "ticket", "reply", "7", "--body", "x", "--email", "--internal"); err == nil {
		t.Fatal("expected error for --email with --internal")
	}
	if _, err := m.runErr(t, "ticket", "reply", "7", "--body", "x", "--to", "a@b.c"); err == nil {
		t.Fatal("expected error for --to without --email")
	}
}

func TestTicketCloseWithNote(t *testing.T) {
	m := newMockZammad(t)
	out := m.run(t, "ticket", "close", "7", "--note", "done")
	if !strings.Contains(out, "Closed ticket #7") {
		t.Errorf("unexpected output: %s", out)
	}
	payload := m.payloads["PUT /api/v1/tickets/7"]
	if !strings.Contains(payload, `"state":"closed"`) || !strings.Contains(payload, `"body":"done"`) {
		t.Errorf("close payload wrong: %s", payload)
	}
}

func TestTicketTags(t *testing.T) {
	m := newMockZammad(t)
	out := m.run(t, "ticket", "tag", "list", "7")
	if !strings.Contains(out, "billing") || !strings.Contains(out, "urgent") {
		t.Errorf("tag list wrong: %s", out)
	}
	m.run(t, "ticket", "tag", "add", "7", "vip")
	if p := m.payloads["POST /api/v1/tags/add"]; !strings.Contains(p, `"item":"vip"`) || !strings.Contains(p, `"o_id":7`) {
		t.Errorf("tag add payload wrong: %s", p)
	}
	m.run(t, "ticket", "tag", "remove", "7", "vip")
	if p := m.payloads["DELETE /api/v1/tags/remove"]; !strings.Contains(p, `"item":"vip"`) {
		t.Errorf("tag remove payload wrong: %s", p)
	}
}

func TestTicketHistory(t *testing.T) {
	m := newMockZammad(t)
	out := m.run(t, "ticket", "history", "7")
	if !strings.Contains(out, "Grace Hopper") || !strings.Contains(out, "new -> open") {
		t.Errorf("history output wrong:\n%s", out)
	}
}

func TestTicketAttachmentsDownload(t *testing.T) {
	m := newMockZammad(t)
	dir := t.TempDir()
	out := m.run(t, "ticket", "attachments", "7", "--download", "--dir", dir)
	if !strings.Contains(out, "fire.log") || !strings.Contains(out, "12 bytes") {
		t.Errorf("download output wrong: %s", out)
	}
}

func TestOverviewListAndView(t *testing.T) {
	m := newMockZammad(t)
	if out := m.run(t, "overview", "list"); !strings.Contains(out, "my_assigned") {
		t.Errorf("overview list wrong: %s", out)
	}
	if out := m.run(t, "overview", "view", "my_assigned"); !strings.Contains(out, "Printer on fire") {
		t.Errorf("overview view wrong: %s", out)
	}
}

func TestGlobalSearch(t *testing.T) {
	m := newMockZammad(t)
	out := m.run(t, "search", "fire")
	if !strings.Contains(out, "Printer on fire") || !strings.Contains(out, "Grace Hopper") || !strings.Contains(out, "open") {
		t.Errorf("global search output wrong:\n%s", out)
	}
}

func TestUserAndOrgSearch(t *testing.T) {
	m := newMockZammad(t)
	if out := m.run(t, "user", "search", "grace"); !strings.Contains(out, "grace@example.com") {
		t.Errorf("user search wrong: %s", out)
	}
	if out := m.run(t, "org", "search", "acme"); !strings.Contains(out, "acme.com") {
		t.Errorf("org search wrong: %s", out)
	}
}

func TestAPIEscapeHatchMultiValue(t *testing.T) {
	m := newMockZammad(t)
	out := m.run(t, "api", "GET", "/api/v1/tickets/search", "-f", "query=a", "-f", "expand=true")
	if !strings.Contains(out, "20001") {
		t.Errorf("api output wrong: %s", out)
	}
}

func TestAuthStatusEnv(t *testing.T) {
	m := newMockZammad(t)
	out := m.run(t, "auth", "status")
	if !strings.Contains(out, "Ada Lovelace") || !strings.Contains(out, "(env)") {
		t.Errorf("auth status wrong: %s", out)
	}
}

func TestInvalidOutputFlag(t *testing.T) {
	m := newMockZammad(t)
	if _, err := m.runErr(t, "ticket", "list", "-o", "yaml"); err == nil {
		t.Fatal("expected error for -o yaml")
	}
}

func TestDocsCommand(t *testing.T) {
	m := newMockZammad(t)
	out := m.run(t, "docs")
	for _, want := range []string{"# zammad CLI reference", "### zammad ticket search", "state.name:open"} {
		if !strings.Contains(out, want) {
			t.Errorf("docs missing %q", want)
		}
	}
}
