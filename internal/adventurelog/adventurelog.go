// Package adventurelog imports trips from an AdventureLog instance: its
// collections become trips, locations become places, lodging and
// transportation map one to one, notes become journal entries and
// checklists come across with their items. Images are downloaded.
package adventurelog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is an authenticated session against one AdventureLog.
type Client struct {
	BaseURL string
	token   string
	HTTP    *http.Client
}

// Login signs in with username + password through the app-token endpoint.
func Login(ctx context.Context, baseURL, username, password string) (*Client, error) {
	c := &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 60 * time.Second}}
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/auth/app/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("adventurelog login failed (HTTP %d)", res.StatusCode)
	}
	var out struct {
		Meta struct {
			SessionToken string `json:"session_token"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Meta.SessionToken == "" {
		return nil, errors.New("adventurelog login returned no session token")
	}
	c.token = out.Meta.SessionToken
	return c, nil
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Session-Token", c.token)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("adventurelog HTTP %d for %s", res.StatusCode, path)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Download fetches a media URL (absolute, same instance) with the session.
func (c *Client) Download(ctx context.Context, u string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("X-Session-Token", c.token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 30<<20))
	return b, res.Header.Get("Content-Type"), err
}

// CollectionSummary is a row of the collections list.
type CollectionSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	IsArchived    bool   `json:"is_archived"`
	LocationCount int    `json:"location_count"`
}

// Collections lists all collections.
func (c *Client) Collections(ctx context.Context) ([]CollectionSummary, error) {
	var raw json.RawMessage
	if err := c.get(ctx, "/api/collections?page_size=1000&order_by=start_date&order_direction=desc", &raw); err != nil {
		return nil, err
	}
	var paged struct {
		Results []CollectionSummary `json:"results"`
	}
	if err := json.Unmarshal(raw, &paged); err == nil && paged.Results != nil {
		return paged.Results, nil
	}
	var list []CollectionSummary
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// Image is a picture attached to something.
type Image struct {
	Image     string `json:"image"`
	IsPrimary bool   `json:"is_primary"`
}

// Attachment is a file attached to something.
type Attachment struct {
	File      string `json:"file"`
	Name      string `json:"name"`
	Extension string `json:"extension"`
}

// Location is an AdventureLog place.
type Location struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Rating      *float64 `json:"rating"`
	Link        *string  `json:"link"`
	Location    *string  `json:"location"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
	IsVisited   bool     `json:"is_visited"`
	Tags        []string `json:"tags"`
	Category    *struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Icon        string `json:"icon"`
	} `json:"category"`
	Visits []struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Notes     string `json:"notes"`
	} `json:"visits"`
	Images      []Image      `json:"images"`
	Attachments []Attachment `json:"attachments"`
	Price       *string      `json:"price"`
	PriceCur    *string      `json:"price_currency"`
}

// Lodging is an AdventureLog stay.
type Lodging struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Type              string   `json:"type"`
	Rating            *float64 `json:"rating"`
	Link              *string  `json:"link"`
	CheckIn           *string  `json:"check_in"`
	CheckOut          *string  `json:"check_out"`
	ReservationNumber *string  `json:"reservation_number"`
	Price             *string  `json:"price"`
	PriceCurrency     *string  `json:"price_currency"`
	Latitude          *float64 `json:"latitude"`
	Longitude         *float64 `json:"longitude"`
	Location          *string  `json:"location"`
	Images            []Image  `json:"images"`
}

// Transportation is an AdventureLog leg.
type Transportation struct {
	ID            string       `json:"id"`
	Type          string       `json:"type"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Link          *string      `json:"link"`
	Date          *string      `json:"date"`
	EndDate       *string      `json:"end_date"`
	FlightNumber  *string      `json:"flight_number"`
	FromLocation  *string      `json:"from_location"`
	ToLocation    *string      `json:"to_location"`
	OriginLat     *float64     `json:"origin_latitude"`
	OriginLng     *float64     `json:"origin_longitude"`
	DestLat       *float64     `json:"destination_latitude"`
	DestLng       *float64     `json:"destination_longitude"`
	Price         *string      `json:"price"`
	PriceCurrency *string      `json:"price_currency"`
	Distance      *float64     `json:"distance"`
	Attachments   []Attachment `json:"attachments"`
}

// Note is an AdventureLog note.
type Note struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Content string   `json:"content"`
	Date    *string  `json:"date"`
	Links   []string `json:"links"`
}

// Checklist is an AdventureLog checklist.
type Checklist struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Items []struct {
		Name    string `json:"name"`
		Checked bool   `json:"is_checked"`
	} `json:"items"`
}

// Collection is a full collection.
type Collection struct {
	CollectionSummary
	Link            *string          `json:"link"`
	Locations       []Location       `json:"locations"`
	Lodging         []Lodging        `json:"lodging"`
	Transportations []Transportation `json:"transportations"`
	Notes           []Note           `json:"notes"`
	Checklists      []Checklist      `json:"checklists"`
}

// Collection loads one collection with everything in it.
func (c *Client) Collection(ctx context.Context, id string) (*Collection, error) {
	var col Collection
	if err := c.get(ctx, "/api/collections/"+id, &col); err != nil {
		return nil, err
	}
	return &col, nil
}
