package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// Ticket holds the fields the CLI renders. With expand=true, Zammad resolves
// relations to human-readable strings (group, state, priority, owner, customer).
type Ticket struct {
	ID          int    `json:"id"`
	Number      string `json:"number"`
	Title       string `json:"title"`
	Group       string `json:"group"`
	GroupID     int    `json:"group_id"`
	State       string `json:"state"`
	Priority    string `json:"priority"`
	Owner       string `json:"owner"`
	OwnerID     int    `json:"owner_id"`
	Customer    string `json:"customer"`
	Note        string `json:"note"`
	ArticleIDs  []int  `json:"article_ids"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	CreatedByID int    `json:"created_by_id"`

	// Raw is the full API object, preserved for --output json.
	Raw json.RawMessage `json:"-"`
}

func expandQuery(extra url.Values) url.Values {
	q := url.Values{"expand": {"true"}}
	for k, vs := range extra {
		q[k] = vs
	}
	return q
}

// SearchTickets queries /api/v1/tickets/search with the given Zammad search
// query (Elasticsearch syntax) and returns up to limit tickets.
func (c *Client) SearchTickets(ctx context.Context, query string, limit int) ([]Ticket, error) {
	perPage := 50
	var all []Ticket
	for page := 1; len(all) < limit; page++ {
		q := expandQuery(url.Values{
			"query":    {query},
			"page":     {strconv.Itoa(page)},
			"per_page": {strconv.Itoa(perPage)},
		})
		batch, err := c.getTickets(ctx, "/api/v1/tickets/search", q)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < perPage {
			break
		}
	}
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// ListTickets returns tickets from /api/v1/tickets (no search filter), newest
// pages first as returned by the server, up to limit.
func (c *Client) ListTickets(ctx context.Context, limit int) ([]Ticket, error) {
	perPage := 50
	if limit < perPage {
		perPage = limit
	}
	var all []Ticket
	for page := 1; len(all) < limit; page++ {
		q := expandQuery(url.Values{
			"page":     {strconv.Itoa(page)},
			"per_page": {strconv.Itoa(perPage)},
		})
		batch, err := c.getTickets(ctx, "/api/v1/tickets", q)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < perPage {
			break
		}
	}
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (c *Client) getTickets(ctx context.Context, path string, q url.Values) ([]Ticket, error) {
	var raw []json.RawMessage
	if err := c.Get(ctx, path, q, &raw); err != nil {
		return nil, err
	}
	tickets := make([]Ticket, 0, len(raw))
	for _, r := range raw {
		var t Ticket
		if err := json.Unmarshal(r, &t); err != nil {
			return nil, fmt.Errorf("decode ticket: %w", err)
		}
		t.Raw = r
		tickets = append(tickets, t)
	}
	return tickets, nil
}

// GetTicket fetches a single ticket with expanded relation names.
func (c *Client) GetTicket(ctx context.Context, id int) (*Ticket, error) {
	var raw json.RawMessage
	if err := c.Get(ctx, "/api/v1/tickets/"+strconv.Itoa(id), expandQuery(nil), &raw); err != nil {
		return nil, err
	}
	var t Ticket
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("decode ticket: %w", err)
	}
	t.Raw = raw
	return &t, nil
}

// TicketArticle is the article payload nested in ticket create/update.
type TicketArticle struct {
	Subject     string              `json:"subject,omitempty"`
	Body        string              `json:"body"`
	Type        string              `json:"type,omitempty"` // note, email, ...
	To          string              `json:"to,omitempty"`   // for type "email": recipient address(es)
	Cc          string              `json:"cc,omitempty"`
	Internal    bool                `json:"internal"`
	ContentType string              `json:"content_type,omitempty"`
	TimeUnit    string              `json:"time_unit,omitempty"` // accounted time, e.g. "15"
	Attachments []ArticleAttachment `json:"attachments,omitempty"`
}

// ArticleAttachment is an attachment on an outgoing article. Note Zammad's
// field name "mime-type" with a hyphen.
type ArticleAttachment struct {
	Filename string `json:"filename"`
	Data     string `json:"data"` // base64
	MimeType string `json:"mime-type"`
}

// TicketCreate is the POST /api/v1/tickets payload. Relation fields accept
// human-readable names (group name, customer email, state/priority names).
type TicketCreate struct {
	Title    string         `json:"title"`
	Group    string         `json:"group"`
	Customer string         `json:"customer"` // email, "guess:<email>", or login
	State    string         `json:"state,omitempty"`
	Priority string         `json:"priority,omitempty"`
	Owner    string         `json:"owner,omitempty"`
	Article  *TicketArticle `json:"article,omitempty"`
}

func (c *Client) CreateTicket(ctx context.Context, in *TicketCreate) (*Ticket, error) {
	var raw json.RawMessage
	if err := c.Do(ctx, http.MethodPost, "/api/v1/tickets", expandQuery(nil), in, &raw); err != nil {
		return nil, err
	}
	var t Ticket
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("decode ticket: %w", err)
	}
	t.Raw = raw
	return &t, nil
}

// TicketUpdate is the PUT /api/v1/tickets/{id} payload. Empty fields are
// omitted. Including Article creates a new article on the ticket.
type TicketUpdate struct {
	Title    string         `json:"title,omitempty"`
	Group    string         `json:"group,omitempty"`
	State    string         `json:"state,omitempty"`
	Priority string         `json:"priority,omitempty"`
	Owner    string         `json:"owner,omitempty"`
	Customer string         `json:"customer,omitempty"`
	Article  *TicketArticle `json:"article,omitempty"`
}

func (c *Client) UpdateTicket(ctx context.Context, id int, in *TicketUpdate) (*Ticket, error) {
	var raw json.RawMessage
	if err := c.Do(ctx, http.MethodPut, "/api/v1/tickets/"+strconv.Itoa(id), expandQuery(nil), in, &raw); err != nil {
		return nil, err
	}
	var t Ticket
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("decode ticket: %w", err)
	}
	t.Raw = raw
	return &t, nil
}
