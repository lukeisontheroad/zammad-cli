package api

import (
	"context"
	"strconv"
)

// Article is a ticket article (message/note) as returned by
// GET /api/v1/ticket_articles/by_ticket/{ticket_id}.
type Article struct {
	ID          int    `json:"id"`
	TicketID    int    `json:"ticket_id"`
	Type        string `json:"type"` // with expand=true: "note", "email", ...
	Sender      string `json:"sender"`
	From        string `json:"from"`
	ReplyTo     string `json:"reply_to"`
	To          string `json:"to"`
	Cc          string `json:"cc"`
	Subject     string `json:"subject"`
	Body        string `json:"body"`
	ContentType string `json:"content_type"`
	Internal    bool   `json:"internal"`
	CreatedAt   string `json:"created_at"`
	Attachments []struct {
		ID       int    `json:"id"`
		Filename string `json:"filename"`
		Size     string `json:"size"`
	} `json:"attachments"`
}

// ListArticles returns all articles of a ticket, oldest first.
func (c *Client) ListArticles(ctx context.Context, ticketID int) ([]Article, error) {
	var articles []Article
	err := c.Get(ctx, "/api/v1/ticket_articles/by_ticket/"+strconv.Itoa(ticketID), expandQuery(nil), &articles)
	return articles, err
}
