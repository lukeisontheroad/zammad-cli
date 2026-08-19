package api

import (
	"context"
	"fmt"
)

// User is a Zammad user (subset of fields).
type User struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Firstname string `json:"firstname"`
	Lastname  string `json:"lastname"`
	Email     string `json:"email"`
}

func (u User) DisplayName() string {
	name := u.Firstname
	if u.Lastname != "" {
		if name != "" {
			name += " "
		}
		name += u.Lastname
	}
	if name == "" {
		name = u.Login
	}
	return name
}

// GetUser fetches a single user by id.
func (c *Client) GetUser(ctx context.Context, id int) (*User, error) {
	var u User
	if err := c.Get(ctx, fmt.Sprintf("/api/v1/users/%d", id), nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// Me returns the authenticated user (also serves as a token check).
func (c *Client) Me(ctx context.Context) (*User, error) {
	var u User
	if err := c.Get(ctx, "/api/v1/users/me", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
