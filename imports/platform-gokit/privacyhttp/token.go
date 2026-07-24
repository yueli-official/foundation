package privacyhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ClientCredentialsTokenSource obtains and briefly caches a machine token for
// Owner protocol calls. The OAuth client and scope are provisioned by Identity;
// Foundation deliberately does not own this authentication policy.
type ClientCredentialsTokenSource struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scope        string
	Client       *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

func (source *ClientCredentialsTokenSource) Token(ctx context.Context) (string, error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	now := time.Now()
	if source.token != "" && now.Before(source.expires.Add(-30*time.Second)) {
		return source.token, nil
	}
	if strings.TrimSpace(source.TokenURL) == "" || strings.TrimSpace(source.ClientID) == "" ||
		strings.TrimSpace(source.ClientSecret) == "" {
		return "", errors.New("privacy owner OAuth client is not configured")
	}
	parsed, err := url.Parse(source.TokenURL)
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback(parsed.Hostname()))) {
		return "", errors.New("privacy owner OAuth token URL must use HTTPS outside loopback")
	}
	form := url.Values{
		"grant_type": {"client_credentials"},
		"scope":      {strings.TrimSpace(source.Scope)},
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, source.TokenURL, strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}
	request.SetBasicAuth(source.ClientID, source.ClientSecret)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := source.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", errors.New("privacy owner OAuth token request was rejected")
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64<<10))
	if err := decoder.Decode(&payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", errors.New("privacy owner OAuth response has no access token")
	}
	if payload.ExpiresIn <= 0 {
		payload.ExpiresIn = 300
	}
	source.token = payload.AccessToken
	source.expires = now.Add(time.Duration(payload.ExpiresIn) * time.Second)
	return source.token, nil
}

func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
