package webhook

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type NetworkPolicy struct {
	AllowHTTP       bool
	AllowedPorts    []uint16
	AllowPrivate    bool
	AllowLoopback   bool
	AllowLinkLocal  bool
	DNSDeadline     time.Duration
	ConnectDeadline time.Duration
	TotalDeadline   time.Duration
	MaxDNSAnswers   int
}

func PublicNetworkPolicy() NetworkPolicy {
	return NetworkPolicy{
		AllowedPorts: []uint16{443}, DNSDeadline: 3 * time.Second,
		ConnectDeadline: 5 * time.Second, TotalDeadline: 20 * time.Second,
		MaxDNSAnswers: 16,
	}
}

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type SystemResolver struct{ Resolver *net.Resolver }

func (resolver SystemResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	value := resolver.Resolver
	if value == nil {
		value = net.DefaultResolver
	}
	return value.LookupNetIP(ctx, network, host)
}

type AuthorizedRoute struct {
	URL       *url.URL
	Address   netip.Addr
	Port      uint16
	ExpiresAt time.Time
}

type NetworkAuthorizer struct {
	Resolver Resolver
	Policy   NetworkPolicy
	Clock    func() time.Time
}

func (authorizer NetworkAuthorizer) Authorize(ctx context.Context, raw string) (AuthorizedRoute, error) {
	policy := authorizer.Policy
	if len(policy.AllowedPorts) == 0 {
		policy = PublicNetworkPolicy()
	}
	canonical, err := ValidateEndpointURL(raw, policy)
	if err != nil {
		return AuthorizedRoute{}, err
	}
	parsed, _ := url.Parse(canonical)
	port := uint16(443)
	if parsed.Scheme == "http" {
		port = 80
	}
	if parsed.Port() != "" {
		value, _ := strconv.ParseUint(parsed.Port(), 10, 16)
		port = uint16(value)
	}
	host := parsed.Hostname()
	var addresses []netip.Addr
	if literal, parseErr := netip.ParseAddr(host); parseErr == nil {
		addresses = []netip.Addr{literal}
	} else {
		resolver := authorizer.Resolver
		if resolver == nil {
			resolver = SystemResolver{}
		}
		deadline := policy.DNSDeadline
		if deadline <= 0 {
			deadline = 3 * time.Second
		}
		resolveContext, cancel := context.WithTimeout(ctx, deadline)
		defer cancel()
		addresses, err = resolver.LookupNetIP(resolveContext, "ip", host)
		if err != nil {
			return AuthorizedRoute{}, &Error{Code: ErrorEndpointUnsafe, Field: "dns", Message: "resolution failed", Retryable: true, Cause: err}
		}
	}
	maxAnswers := policy.MaxDNSAnswers
	if maxAnswers == 0 {
		maxAnswers = 16
	}
	if len(addresses) == 0 || len(addresses) > maxAnswers {
		return AuthorizedRoute{}, invalid(ErrorEndpointUnsafe, "dns", "answer count is invalid")
	}
	for _, address := range addresses {
		if err := authorizeAddress(address, policy); err != nil {
			return AuthorizedRoute{}, err
		}
	}
	now := time.Now().UTC()
	if authorizer.Clock != nil {
		now = authorizer.Clock().UTC()
	}
	return AuthorizedRoute{URL: parsed, Address: addresses[0].Unmap(), Port: port, ExpiresAt: now.Add(time.Minute)}, nil
}

var deniedAddressPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("ff00::/8"),
}

func authorizeAddress(input netip.Addr, policy NetworkPolicy) error {
	address := input.Unmap()
	if !address.IsValid() || address.IsUnspecified() || address.IsMulticast() {
		return invalid(ErrorEndpointUnsafe, "address", "resolved to a prohibited address")
	}
	if address.IsLoopback() && !policy.AllowLoopback {
		return invalid(ErrorEndpointUnsafe, "address", "resolved to a prohibited address")
	}
	if address.IsLinkLocalUnicast() && !policy.AllowLinkLocal {
		return invalid(ErrorEndpointUnsafe, "address", "resolved to a prohibited address")
	}
	if address.IsPrivate() && !policy.AllowPrivate {
		return invalid(ErrorEndpointUnsafe, "address", "resolved to a prohibited address")
	}
	for _, prefix := range deniedAddressPrefixes {
		if prefix.Contains(address) {
			return invalid(ErrorEndpointUnsafe, "address", "resolved to a prohibited address")
		}
	}
	return nil
}

type ExchangeLimits struct {
	Timeout          time.Duration
	MaxResponseBytes int64
	MaxHeaderBytes   int64
}

type OutboundRequest struct {
	Route  AuthorizedRoute
	Header http.Header
	Body   []byte
	Limits ExchangeLimits
}

type SendResult struct {
	StatusCode     int
	ResponseDigest string
	RetryAfter     time.Duration
	StartedAt      time.Time
	FinishedAt     time.Time
	RemoteAddress  string
}

type Sender interface {
	Send(context.Context, OutboundRequest) (SendResult, error)
}

type HTTPSender struct {
	Clock func() time.Time
}

func (sender HTTPSender) Send(ctx context.Context, request OutboundRequest) (SendResult, error) {
	now := time.Now().UTC()
	if sender.Clock != nil {
		now = sender.Clock().UTC()
	}
	if request.Route.URL == nil || !request.Route.Address.IsValid() || now.After(request.Route.ExpiresAt) {
		return SendResult{}, invalid(ErrorEndpointUnsafe, "route", "authorization is missing or expired")
	}
	timeout := request.Limits.Timeout
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 20 * time.Second
	}
	maxBody := request.Limits.MaxResponseBytes
	if maxBody <= 0 || maxBody > 1<<20 {
		maxBody = 64 << 10
	}
	maxHeaders := request.Limits.MaxHeaderBytes
	if maxHeaders <= 0 || maxHeaders > 1<<20 {
		maxHeaders = 64 << 10
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	target := request.Route.URL
	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodPost, target.String(), strings.NewReader(string(request.Body)))
	if err != nil {
		return SendResult{}, unavailable("construct request", err)
	}
	httpRequest.Header = request.Header.Clone()
	httpRequest.Header.Del("Authorization")
	httpRequest.Header.Del("Cookie")
	httpRequest.Header.Del("Proxy-Authorization")
	httpRequest.Header.Del("Forwarded")
	httpRequest.Header.Del("X-Forwarded-For")
	httpRequest.Header.Set("Accept-Encoding", "identity")
	httpRequest.Host = target.Host
	dialer := &net.Dialer{Timeout: min(timeout, 5*time.Second), KeepAlive: 30 * time.Second}
	pinned := net.JoinHostPort(request.Route.Address.String(), strconv.Itoa(int(request.Route.Port)))
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(dialContext context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(dialContext, network, pinned)
		},
		ForceAttemptHTTP2:      true,
		DisableCompression:     true,
		MaxIdleConns:           10,
		MaxIdleConnsPerHost:    2,
		IdleConnTimeout:        30 * time.Second,
		TLSHandshakeTimeout:    min(timeout, 5*time.Second),
		ResponseHeaderTimeout:  min(timeout, 10*time.Second),
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: maxHeaders,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: target.Hostname(),
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport, Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	started := now
	response, err := client.Do(httpRequest)
	finished := time.Now().UTC()
	if sender.Clock != nil {
		finished = sender.Clock().UTC()
	}
	if err != nil {
		return SendResult{StartedAt: started, FinishedAt: finished, RemoteAddress: redactAddress(request.Route.Address)}, &Error{
			Code: ErrorUnavailable, Field: "transport", Message: "request failed", Retryable: true, Cause: err,
		}
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return SendResult{StatusCode: response.StatusCode, StartedAt: started, FinishedAt: finished}, &Error{
			Code: ErrorUnavailable, Field: "response", Message: "cannot read bounded response", Retryable: true, Cause: err,
		}
	}
	if int64(len(content)) > maxBody {
		return SendResult{StatusCode: response.StatusCode, StartedAt: started, FinishedAt: finished}, &Error{
			Code: ErrorLimitExceeded, Field: "response", Message: "response exceeds limit", Retryable: true,
		}
	}
	sum := sha256.Sum256(content)
	return SendResult{
		StatusCode: response.StatusCode, ResponseDigest: hex.EncodeToString(sum[:]),
		RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), finished),
		StartedAt:  started, FinishedAt: finished, RemoteAddress: redactAddress(request.Route.Address),
	}, nil
}

func parseRetryAfter(raw string, now time.Time) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if instant, err := http.ParseTime(raw); err == nil && instant.After(now) {
		return instant.Sub(now)
	}
	return 0
}

func redactAddress(address netip.Addr) string {
	sum := sha256.Sum256([]byte(address.String()))
	return fmt.Sprintf("sha256:%s", hex.EncodeToString(sum[:8]))
}
