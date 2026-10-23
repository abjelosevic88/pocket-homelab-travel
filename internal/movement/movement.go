// Package movement reads Pocket Movement, the person's own location history.
// Movement speaks Dawarich's API (points, timeline, visits), so the route and
// the timeline come through the dawarich client; on top of that it keeps a
// summary per day (distance from its journeys, the countries you were in),
// which is a better source for distances and countries than raw points.
// Travel only ever reaches it through Pocket Core, with a link token.
package movement

import (
	"context"
	"net/url"
	"time"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/dawarich"
)

// Client is one person's Pocket Movement, for one piece of work (the link
// token it carries lives five minutes).
type Client struct {
	*dawarich.Client
}

// New creates a client: baseURL is Movement's address from Core, token the
// pcx_ link token (Movement takes it wherever Dawarich takes an api_key).
func New(baseURL, token string, timeout time.Duration) *Client {
	c := dawarich.New(baseURL, token, timeout)
	c.Name = "pocket movement"
	c.DistanceUnit = "km"
	return &Client{Client: c}
}

// Day is Movement's summary of one day.
type Day struct {
	Date      string             `json:"date"`
	Points    int                `json:"points"`
	Distance  float64            `json:"distance"` // metres
	MovingS   int64              `json:"moving_s"`
	Countries []string           `json:"countries"` // ISO 3166-1 alpha-2, in order of entry
	Modes     map[string]float64 `json:"modes"`     // metres per mode
	Visits    int                `json:"visits"`
}

// Days returns the day summaries between two dates (YYYY-MM-DD, inclusive).
func (c *Client) Days(ctx context.Context, from, to string) ([]Day, error) {
	var out []Day
	if err := c.GetJSON(ctx, "/api/v1/calendar", url.Values{"from": {from}, "to": {to}}, &out); err != nil {
		return nil, err
	}
	return out, nil
}
