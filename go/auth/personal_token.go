package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PersonalScope binds one explicit permission to one independently deployed
// consumer. Wildcards and role names must never be expanded during use.
func PersonalScope(site, capability string) (string, error) {
	if !validPermissionPart(site) || !validPermissionPart(capability) {
		return "", ErrInvalidClaims
	}
	return "site:" + base64.RawURLEncoding.EncodeToString([]byte(site)) + ":" + capability, nil
}

func ParsePersonalScope(scope string) (site, capability string, ok bool) {
	parts := strings.Split(scope, ":")
	if len(parts) != 3 || parts[0] != "site" {
		return "", "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", false
	}
	canonical, err := PersonalScope(string(raw), parts[2])
	return string(raw), parts[2], err == nil && canonical == scope
}

func validPermissionPart(value string) bool {
	if value == "" || len(value) > 200 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._-", c)) {
			return false
		}
	}
	return true
}

// TokenVerifier is shared by transports and composite authenticators.
type TokenVerifier interface {
	Verify(context.Context, string) (*Principal, error)
}

// PersonalTokenVerifier verifies opaque PATs online on every request. There is
// deliberately no positive cache: deletion and account disable take effect on
// the next verification. Consumer authorization must still run for each action.
type PersonalTokenVerifier struct {
	endpoint string
	site     string
	client   *http.Client
}

// PersonalTransportOptions is deployment-owned. AllowHTTP is only for an
// explicitly trusted private service network; HTTPS is the default.
type PersonalTransportOptions struct{ AllowHTTP bool }

func NewPersonalTokenVerifier(endpoint, site string, client *http.Client, options ...PersonalTransportOptions) (*PersonalTokenVerifier, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !validPermissionPart(site) {
		return nil, ErrInvalidClaims
	}
	loopback := u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" || u.Hostname() == "localhost"
	allowHTTP := len(options) == 1 && options[0].AllowHTTP
	if len(options) > 1 || u.Scheme != "https" && !(u.Scheme == "http" && (loopback || allowHTTP)) {
		return nil, ErrInvalidClaims
	}
	configured := http.Client{Timeout: 5 * time.Second}
	if client != nil {
		configured = *client
	}
	// Never forward a user's secret to a redirect target, including same-host
	// redirects whose destination might be another service or logging endpoint.
	configured.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &PersonalTokenVerifier{endpoint: endpoint, site: site, client: &configured}, nil
}

func (v *PersonalTokenVerifier) Verify(ctx context.Context, token string) (*Principal, error) {
	if v == nil || !strings.HasPrefix(token, "pat_") || len(token) > 1024 || strings.ContainsAny(token, " \r\n\t") {
		return nil, ErrMalformedToken
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, nil)
	if err != nil {
		return nil, ErrMalformedToken
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := v.client.Do(req)
	if err != nil {
		return nil, errors.New("auth: personal token verification unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ErrInvalidClaims
	}
	var result struct {
		UserKey   string     `json:"userKey"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	}
	decoder := json.NewDecoder(io.LimitReader(res.Body, 64<<10))
	if err := decoder.Decode(&result); err != nil || !validPermissionPart(result.UserKey) || len(result.Scopes) > 50 {
		return nil, ErrInvalidClaims
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, ErrInvalidClaims
	}
	if result.ExpiresAt != nil && !result.ExpiresAt.After(time.Now()) {
		return nil, ErrExpired
	}
	permissions := make([]string, 0)
	for _, scope := range result.Scopes {
		site, capability, ok := ParsePersonalScope(scope)
		if ok && site == v.site {
			permissions = append(permissions, capability)
		}
	}
	if len(permissions) == 0 {
		return nil, ErrInvalidAudience
	}
	p := &Principal{Subject: result.UserKey, SubjectKind: SubjectUser, ClientID: v.site, Audience: []string{v.site}, Scopes: permissions, Roles: []string{}, personalToken: true, claims: map[string]any{"subject_kind": "user"}}
	if result.ExpiresAt != nil {
		p.ExpiresAt = *result.ExpiresAt
	}
	return p, nil
}

// CompositeVerifier preserves JWT verification and only routes the reserved
// PAT prefix to the online verifier. Failed PATs never fall back to JWT auth.
type CompositeVerifier struct {
	JWT      TokenVerifier
	Personal *PersonalTokenVerifier
}

func (v CompositeVerifier) Verify(ctx context.Context, raw string) (*Principal, error) {
	if strings.HasPrefix(raw, "pat_") {
		return v.Personal.Verify(ctx, raw)
	}
	if v.JWT == nil {
		return nil, ErrMalformedToken
	}
	return v.JWT.Verify(ctx, raw)
}

func (p *Principal) IsPersonalToken() bool { return p != nil && p.personalToken }

// AllowsPersonalCapability adds a restriction only for PAT authentication.
// It never grants the user's underlying business permission.
func AllowsPersonalCapability(ctx context.Context, capability string) bool {
	p, _ := FromContext(ctx)
	return p == nil || !p.IsPersonalToken() || p.HasScope(capability)
}
