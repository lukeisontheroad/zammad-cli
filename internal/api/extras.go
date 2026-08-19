package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Download fetches a raw (non-JSON) resource, e.g. an attachment.
func (c *Client) Download(ctx context.Context, path string) ([]byte, error) {
	u := *c.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token token="+c.token)
	req.Header.Set("User-Agent", "zammad-cli")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		apiErr := &Error{StatusCode: resp.StatusCode}
		_ = json.Unmarshal(data, apiErr)
		return nil, apiErr
	}
	return data, nil
}

// DownloadAttachment fetches one attachment's bytes.
func (c *Client) DownloadAttachment(ctx context.Context, ticketID, articleID, attachmentID int) ([]byte, error) {
	return c.Download(ctx, fmt.Sprintf("/api/v1/ticket_attachment/%d/%d/%d", ticketID, articleID, attachmentID))
}

// SearchUsers queries /api/v1/users/search.
func (c *Client) SearchUsers(ctx context.Context, query string, limit int) ([]User, error) {
	var users []User
	err := c.Get(ctx, "/api/v1/users/search", url.Values{
		"query": {query},
		"limit": {strconv.Itoa(limit)},
	}, &users)
	return users, err
}

// Organization is a Zammad organization (subset of fields).
type Organization struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Note   string `json:"note"`
	Domain string `json:"domain"`
	Active bool   `json:"active"`
}

// SearchOrganizations queries /api/v1/organizations/search.
func (c *Client) SearchOrganizations(ctx context.Context, query string, limit int) ([]Organization, error) {
	var orgs []Organization
	err := c.Get(ctx, "/api/v1/organizations/search", url.Values{
		"query": {query},
		"limit": {strconv.Itoa(limit)},
	}, &orgs)
	return orgs, err
}

// TicketTags lists the tags of a ticket.
func (c *Client) TicketTags(ctx context.Context, ticketID int) ([]string, error) {
	var resp struct {
		Tags []string `json:"tags"`
	}
	err := c.Get(ctx, "/api/v1/tags", url.Values{
		"object": {"Ticket"},
		"o_id":   {strconv.Itoa(ticketID)},
	}, &resp)
	return resp.Tags, err
}

type tagPayload struct {
	Item   string `json:"item"`
	Object string `json:"object"`
	OID    int    `json:"o_id"`
}

func (c *Client) AddTicketTag(ctx context.Context, ticketID int, tag string) error {
	return c.Do(ctx, http.MethodPost, "/api/v1/tags/add", nil, tagPayload{tag, "Ticket", ticketID}, nil)
}

func (c *Client) RemoveTicketTag(ctx context.Context, ticketID int, tag string) error {
	// Zammad expects a JSON body on this DELETE.
	return c.Do(ctx, http.MethodDelete, "/api/v1/tags/remove", nil, tagPayload{tag, "Ticket", ticketID}, nil)
}

// HistoryEntry is one row of a ticket's change history.
type HistoryEntry struct {
	Object      string `json:"object"` // Ticket, Ticket::Article, Tag, Mention, ...
	Type        string `json:"type"`   // created, updated, ...
	Attribute   string `json:"attribute"`
	ValueFrom   any    `json:"value_from"`
	ValueTo     any    `json:"value_to"`
	CreatedByID int    `json:"created_by_id"`
	CreatedAt   string `json:"created_at"`
}

// TicketHistory returns a ticket's history entries plus a user id -> display
// name map resolved from the response assets (no extra requests needed).
func (c *Client) TicketHistory(ctx context.Context, ticketID int) ([]HistoryEntry, map[int]string, error) {
	var resp struct {
		History []HistoryEntry `json:"history"`
		Assets  struct {
			User map[string]User `json:"User"`
		} `json:"assets"`
	}
	if err := c.Get(ctx, "/api/v1/ticket_history/"+strconv.Itoa(ticketID), nil, &resp); err != nil {
		return nil, nil, err
	}
	users := map[int]string{}
	for _, u := range resp.Assets.User {
		users[u.ID] = u.DisplayName()
	}
	return resp.History, users, nil
}

// SystemEmailAddresses returns all email addresses owned by the Zammad
// instance itself (group inboxes), used to exclude them from reply-all CCs.
func (c *Client) SystemEmailAddresses(ctx context.Context) ([]string, error) {
	var list []struct {
		Email string `json:"email"`
	}
	if err := c.Get(ctx, "/api/v1/email_addresses", nil, &list); err != nil {
		return nil, err
	}
	addrs := make([]string, 0, len(list))
	for _, e := range list {
		addrs = append(addrs, strings.ToLower(e.Email))
	}
	return addrs, nil
}

// Signature is an email signature configured in Zammad. Body is HTML and may
// contain #{user.attribute} placeholders, which the Zammad UI substitutes
// client-side — API clients must render them themselves.
type Signature struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Body     string `json:"body"`
	Active   bool   `json:"active"`
	GroupIDs []int  `json:"group_ids"`
}

// Signatures lists all configured email signatures.
func (c *Client) Signatures(ctx context.Context) ([]Signature, error) {
	var sigs []Signature
	err := c.Get(ctx, "/api/v1/signatures", nil, &sigs)
	return sigs, err
}

// GroupSignature returns the active signature assigned to a group, or nil if
// none is configured.
func (c *Client) GroupSignature(ctx context.Context, groupID int) (*Signature, error) {
	sigs, err := c.Signatures(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range sigs {
		if !s.Active {
			continue
		}
		for _, id := range s.GroupIDs {
			if id == groupID {
				return &s, nil
			}
		}
	}
	return nil, nil
}

// MeRaw returns the authenticated user's full profile as a generic map,
// used for signature placeholder substitution.
func (c *Client) MeRaw(ctx context.Context) (map[string]any, error) {
	var m map[string]any
	if err := c.Get(ctx, "/api/v1/users/me", nil, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// Overview is a saved ticket overview (the left sidebar in the Zammad UI).
type Overview struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Link  string `json:"link"`
	Count int    `json:"count"`
}

// ListOverviews returns the user's overviews with ticket counts.
func (c *Client) ListOverviews(ctx context.Context) ([]Overview, error) {
	var overviews []Overview
	err := c.Get(ctx, "/api/v1/ticket_overviews", nil, &overviews)
	return overviews, err
}

// OverviewTicketIDs returns the ticket ids in one overview (by its link),
// in the overview's configured order.
func (c *Client) OverviewTicketIDs(ctx context.Context, link string) ([]int, error) {
	var resp struct {
		Index struct {
			Tickets []struct {
				ID int `json:"id"`
			} `json:"tickets"`
		} `json:"index"`
	}
	if err := c.Get(ctx, "/api/v1/ticket_overviews", url.Values{"view": {link}}, &resp); err != nil {
		return nil, err
	}
	ids := make([]int, len(resp.Index.Tickets))
	for i, t := range resp.Index.Tickets {
		ids[i] = t.ID
	}
	return ids, nil
}

// SearchResult is one hit of the global search, with display fields resolved
// from the response assets.
type SearchResult struct {
	Type  string // Ticket, User, Organization, ...
	ID    int
	Label string // ticket title, user name, org name
	Extra string // ticket state / user email / org domain
}

// GlobalSearch queries /api/v1/search across all searchable objects.
func (c *Client) GlobalSearch(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	var resp struct {
		Result []struct {
			Type string `json:"type"`
			ID   int    `json:"id"`
		} `json:"result"`
		Assets map[string]map[string]json.RawMessage `json:"assets"`
	}
	if err := c.Get(ctx, "/api/v1/search", url.Values{
		"query": {query},
		"limit": {strconv.Itoa(limit)},
	}, &resp); err != nil {
		return nil, err
	}

	// State names for ticket hits; ignore errors (extra info only).
	states := map[int]string{}
	var stateList []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := c.Get(ctx, "/api/v1/ticket_states", nil, &stateList); err == nil {
		for _, s := range stateList {
			states[s.ID] = s.Name
		}
	}

	results := make([]SearchResult, 0, len(resp.Result))
	for _, r := range resp.Result {
		res := SearchResult{Type: r.Type, ID: r.ID}
		if raw, ok := resp.Assets[r.Type][strconv.Itoa(r.ID)]; ok {
			switch r.Type {
			case "Ticket":
				var t struct {
					Title   string `json:"title"`
					StateID int    `json:"state_id"`
				}
				if json.Unmarshal(raw, &t) == nil {
					res.Label = t.Title
					res.Extra = states[t.StateID]
				}
			case "User":
				var u User
				if json.Unmarshal(raw, &u) == nil {
					res.Label = u.DisplayName()
					res.Extra = u.Email
				}
			case "Organization":
				var o Organization
				if json.Unmarshal(raw, &o) == nil {
					res.Label = o.Name
					res.Extra = o.Domain
				}
			}
		}
		results = append(results, res)
	}
	return results, nil
}
