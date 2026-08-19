package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, "testtoken")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAuthHeaderAndJSON(t *testing.T) {
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Token token=testtoken" {
			t.Errorf("bad auth header: %q", got)
		}
		if r.URL.Path != "/api/v1/users/me" {
			t.Errorf("bad path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":1,"login":"agent","firstname":"Ada","lastname":"Lovelace","email":"ada@example.com"}`)
	})
	me, err := c.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if me.Login != "agent" || me.DisplayName() != "Ada Lovelace" {
		t.Fatalf("unexpected user: %+v", me)
	}
}

func TestErrorMapping(t *testing.T) {
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"Invalid token","error_human":"The token is invalid."}`)
	})
	_, err := c.Me(context.Background())
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 401 || apiErr.Human != "The token is invalid." {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
}

func TestErrorNonJSONBody(t *testing.T) {
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, "<html>proxy error</html>")
	})
	_, err := c.Me(context.Background())
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.StatusCode != 502 {
		t.Fatalf("unexpected status: %d", apiErr.StatusCode)
	}
}

func TestSearchTicketsPagination(t *testing.T) {
	pages := map[string]string{
		"1": ticketsJSON(1, 50),
		"2": ticketsJSON(51, 20),
	}
	var gotQueries []string
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tickets/search" {
			t.Errorf("bad path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("expand") != "true" {
			t.Error("expand=true missing")
		}
		gotQueries = append(gotQueries, r.URL.Query().Get("query"))
		fmt.Fprint(w, pages[r.URL.Query().Get("page")])
	})
	tickets, err := c.SearchTickets(context.Background(), "state.name:open", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 70 {
		t.Fatalf("expected 70 tickets, got %d", len(tickets))
	}
	if len(gotQueries) != 2 || gotQueries[0] != "state.name:open" {
		t.Fatalf("unexpected queries: %v", gotQueries)
	}
	if tickets[0].ID != 1 || tickets[69].ID != 70 {
		t.Fatalf("unexpected ids: %d %d", tickets[0].ID, tickets[69].ID)
	}
	if len(tickets[0].Raw) == 0 {
		t.Fatal("Raw not preserved")
	}
}

func TestSearchTicketsLimit(t *testing.T) {
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, ticketsJSON(1, 50))
	})
	tickets, err := c.SearchTickets(context.Background(), "x", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 5 {
		t.Fatalf("expected limit 5, got %d", len(tickets))
	}
}

func TestUpdateTicketPayload(t *testing.T) {
	var body map[string]any
	c := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/tickets/42" {
			t.Errorf("bad request: %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("bad content type: %s", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		fmt.Fprint(w, `{"id":42,"state":"closed","title":"t"}`)
	})
	_, err := c.UpdateTicket(context.Background(), 42, &TicketUpdate{
		State:   "closed",
		Article: &TicketArticle{Body: "done", Type: "note", Internal: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["state"] != "closed" {
		t.Fatalf("state not sent: %v", body)
	}
	if _, ok := body["title"]; ok {
		t.Fatal("empty title should be omitted")
	}
	article, _ := body["article"].(map[string]any)
	if article == nil || article["internal"] != true || article["body"] != "done" {
		t.Fatalf("article payload wrong: %v", body["article"])
	}
}

func ticketsJSON(startID, n int) string {
	out := "["
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprintf(`{"id":%d,"title":"Ticket %d","state":"open","priority":"2 normal","group":"Support","owner":"agent","updated_at":"2026-08-19T10:00:00Z"}`, startID+i, startID+i)
	}
	return out + "]"
}
