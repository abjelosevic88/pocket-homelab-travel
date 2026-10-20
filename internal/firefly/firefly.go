// Package firefly reads spending through the Firefly III API: Pocket Money's
// Firefly-compatible API (<money>/firefly) or a real Firefly III. A trip is
// linked to a tag; withdrawals carrying that tag are what was actually spent.
package firefly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to one Firefly III instance.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New creates a client.
func New(baseURL, token string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: timeout}}
}

// Configured reports whether URL and token are set.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" && c.Token != "" }

// Tag is a Firefly tag.
type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Transaction is one split of a withdrawal.
type Transaction struct {
	ID          string   `json:"id"` // transaction_journal_id
	GroupID     string   `json:"group_id"`
	Date        string   `json:"date"` // YYYY-MM-DD
	Description string   `json:"description"`
	Amount      float64  `json:"amount"`
	Currency    string   `json:"currency"`
	Decimals    int      `json:"decimals"`
	ForeignAmt  float64  `json:"foreign_amount"`
	ForeignCur  string   `json:"foreign_currency"`
	Category    string   `json:"category"`
	Budget      string   `json:"budget"`
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Notes       string   `json:"notes"`
	Tags        []string `json:"tags"`
	Type        string   `json:"type"`
	// ExternalID is the split's external_id. Pocket Money fills it with
	// "firefly:<journal id>" for transactions it mirrored from Firefly, which
	// lets a cost synced from Firefly be recognised when it comes from Money.
	ExternalID string `json:"external_id"`
	// FromMoney: the transaction is served by Pocket Money (its
	// original_source is "pocket-money-…"), so ID is a Money id.
	FromMoney bool `json:"from_money"`
}

// FireflyOrigin returns the Firefly journal id a Pocket Money transaction was
// mirrored from ("" when it says nothing about one). Only meaningful for a
// transaction served by Pocket Money.
func (t Transaction) FireflyOrigin() string {
	if id, ok := strings.CutPrefix(t.ExternalID, "firefly:"); ok {
		return id
	}
	return ""
}

func (c *Client) do(ctx context.Context, path string, q url.Values, v any) error {
	if !c.Configured() {
		return errors.New("no URL or token")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.api+json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errors.New("the token was rejected (401)")
	}
	if res.StatusCode == http.StatusForbidden {
		return errors.New("the token may not read this (403)")
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", res.StatusCode, path)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// About returns the Firefly version (connection test).
func (c *Client) About(ctx context.Context) (string, error) {
	var out struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := c.do(ctx, "/api/v1/about", nil, &out); err != nil {
		return "", err
	}
	return out.Data.Version, nil
}

// Describe names what answers the API ("Pocket Money 0.4.0" or
// "Firefly III 6.1.0"), for the connection test.
func (c *Client) Describe(ctx context.Context) (string, error) {
	var out struct {
		Data struct {
			Version    string `json:"version"`
			PHPVersion string `json:"php_version"`
		} `json:"data"`
	}
	if err := c.do(ctx, "/api/v1/about", nil, &out); err != nil {
		return "", err
	}
	if v, ok := strings.CutPrefix(out.Data.PHPVersion, "pocket-money"); ok {
		return strings.TrimSpace("Pocket Money " + strings.TrimSpace(v)), nil
	}
	return "Firefly III " + out.Data.Version, nil
}

// Tags lists all tags.
func (c *Client) Tags(ctx context.Context) ([]Tag, error) {
	var out []Tag
	for page := 1; page < 50; page++ {
		var res struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Tag string `json:"tag"`
				} `json:"attributes"`
			} `json:"data"`
			Meta struct {
				Pagination struct {
					TotalPages int `json:"total_pages"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		q := url.Values{"limit": {"100"}, "page": {strconv.Itoa(page)}}
		if err := c.do(ctx, "/api/v1/tags", q, &res); err != nil {
			return nil, err
		}
		for _, d := range res.Data {
			out = append(out, Tag{ID: d.ID, Name: d.Attributes.Tag})
		}
		if page >= res.Meta.Pagination.TotalPages {
			break
		}
	}
	return out, nil
}

// TagByName finds a tag id (case-insensitive).
func (c *Client) TagByName(ctx context.Context, name string) (*Tag, error) {
	tags, err := c.Tags(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range tags {
		if strings.EqualFold(t.Name, name) {
			return &t, nil
		}
	}
	return nil, nil
}

type txPage struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			Transactions []struct {
				JournalID       string   `json:"transaction_journal_id"`
				Type            string   `json:"type"`
				Date            string   `json:"date"`
				Description     string   `json:"description"`
				Amount          string   `json:"amount"`
				CurrencyCode    string   `json:"currency_code"`
				CurrencyDec     int      `json:"currency_decimal_places"`
				ForeignAmount   *string  `json:"foreign_amount"`
				ForeignCurrency *string  `json:"foreign_currency_code"`
				CategoryName    *string  `json:"category_name"`
				BudgetName      *string  `json:"budget_name"`
				SourceName      *string  `json:"source_name"`
				DestinationName *string  `json:"destination_name"`
				Notes           *string  `json:"notes"`
				Tags            []string `json:"tags"`
				ExternalID      *string  `json:"external_id"`
				OriginalSource  *string  `json:"original_source"`
			} `json:"transactions"`
		} `json:"attributes"`
	} `json:"data"`
	Meta struct {
		Pagination struct {
			TotalPages int `json:"total_pages"`
		} `json:"pagination"`
	} `json:"meta"`
}

// TagTransactions returns the withdrawals carrying a tag between two dates (inclusive).
func (c *Client) TagTransactions(ctx context.Context, tagID, start, end string) ([]Transaction, error) {
	var out []Transaction
	for page := 1; page < 100; page++ {
		var res txPage
		q := url.Values{"limit": {"100"}, "page": {strconv.Itoa(page)}, "type": {"withdrawal"}}
		if start != "" {
			q.Set("start", start)
		}
		if end != "" {
			q.Set("end", end)
		}
		if err := c.do(ctx, "/api/v1/tags/"+url.PathEscape(tagID)+"/transactions", q, &res); err != nil {
			return nil, err
		}
		for _, g := range res.Data {
			for _, t := range g.Attributes.Transactions {
				if t.Type != "withdrawal" {
					continue
				}
				amt, _ := strconv.ParseFloat(t.Amount, 64)
				tx := Transaction{ID: t.JournalID, GroupID: g.ID, Date: t.Date[:min(10, len(t.Date))], Description: t.Description, Amount: amt, Currency: t.CurrencyCode, Decimals: t.CurrencyDec, Tags: t.Tags, Type: t.Type}
				if t.ForeignAmount != nil && t.ForeignCurrency != nil {
					tx.ForeignAmt, _ = strconv.ParseFloat(*t.ForeignAmount, 64)
					tx.ForeignCur = *t.ForeignCurrency
				}
				tx.Category = deref(t.CategoryName)
				tx.Budget = deref(t.BudgetName)
				tx.Source = deref(t.SourceName)
				tx.Destination = deref(t.DestinationName)
				tx.Notes = deref(t.Notes)
				tx.ExternalID = deref(t.ExternalID)
				tx.FromMoney = strings.HasPrefix(deref(t.OriginalSource), "pocket-money")
				out = append(out, tx)
			}
		}
		if page >= res.Meta.Pagination.TotalPages {
			break
		}
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
