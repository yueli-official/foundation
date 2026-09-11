package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const PersonalPermissionsScope = "personal-token:permissions"

// PersonalApplication is trusted registration metadata, never user input or a
// replica of consumer roles. ID is an immutable deployment/client identifier.
type PersonalApplication struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	PermissionsURL string `json:"permissionsUrl"`
	Audience       string `json:"audience"`
}

type PersonalPermission struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type PersonalPermissions struct {
	Site    string               `json:"site"`
	UserKey string               `json:"userKey"`
	Items   []PersonalPermission `json:"items"`
}

// PersonalCatalog queries consumer-owned authorization using an authenticated
// control-plane credential. Results are deliberately not cached as grants.
type PersonalCatalog struct {
	applications []PersonalApplication
	mint         func(context.Context, string) (string, error)
	client       *http.Client
}

func NewPersonalCatalog(applications []PersonalApplication, mint func(context.Context, string) (string, error), client *http.Client, options ...PersonalTransportOptions) (*PersonalCatalog, error) {
	if len(applications) > 100 || mint == nil {
		return nil, ErrInvalidClaims
	}
	seen := map[string]bool{}
	for _, app := range applications {
		if seen[app.ID] || strings.TrimSpace(app.Name) == "" || len(app.Name) > 200 || app.Audience == "" {
			return nil, ErrInvalidClaims
		}
		if _, err := NewPersonalTokenVerifier(app.PermissionsURL, app.ID, nil, options...); err != nil {
			return nil, fmt.Errorf("invalid personal application %q: %w", app.ID, err)
		}
		seen[app.ID] = true
	}
	httpClient := http.Client{Timeout: 5 * time.Second}
	if client != nil {
		httpClient = *client
	}
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &PersonalCatalog{applications: append([]PersonalApplication{}, applications...), mint: mint, client: &httpClient}, nil
}

func (c *PersonalCatalog) Applications() []PersonalApplication {
	if c == nil {
		return []PersonalApplication{}
	}
	return append([]PersonalApplication{}, c.applications...)
}

func (c *PersonalCatalog) Permissions(ctx context.Context, site, userKey string) ([]PersonalPermission, error) {
	if c == nil || userKey == "" {
		return nil, ErrInvalidClaims
	}
	var selected *PersonalApplication
	for i := range c.applications {
		if c.applications[i].ID == site {
			selected = &c.applications[i]
			break
		}
	}
	if selected == nil {
		return nil, ErrInvalidAudience
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	token, err := c.mint(ctx, selected.Audience)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(selected.PermissionsURL)
	u.RawQuery = url.Values{"userKey": []string{userKey}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, ErrInvalidClaims
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("personal application %q unavailable", site)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("personal application %q unavailable", site)
	}
	var result PersonalPermissions
	d := json.NewDecoder(io.LimitReader(res.Body, 64<<10))
	if err := d.Decode(&result); err != nil || d.Decode(new(any)) != io.EOF || result.Site != site || result.UserKey != userKey || len(result.Items) > 50 {
		return nil, ErrInvalidClaims
	}
	seen := map[string]bool{}
	for _, item := range result.Items {
		if !validPermissionPart(item.Key) || seen[item.Key] || item.Label == "" || len(item.Label) > 200 || len(item.Description) > 2000 {
			return nil, ErrInvalidClaims
		}
		seen[item.Key] = true
	}
	return result.Items, nil
}
