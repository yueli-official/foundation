// Package httpadapter transports privacy Owner commands without changing the
// core protocol. Authentication and service discovery remain consumer-owned.
package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/yueli-official/foundation/go/privacy"
)

type TokenSource func(context.Context) (string, error)

type ClientOptions struct {
	Endpoint    string
	Client      *http.Client
	TokenSource TokenSource
	MaxBytes    int64
	// AllowInsecureHTTP is an explicit compatibility escape hatch for an
	// isolated service network that does not yet terminate TLS per workload.
	// It must never be enabled for public or untrusted networks.
	AllowInsecureHTTP bool
}

type Client struct {
	endpoint    string
	client      *http.Client
	tokenSource TokenSource
	maxBytes    int64
}

var _ privacy.OwnerHost = (*Client)(nil)

func NewClient(options ClientOptions) (*Client, error) {
	endpoint := strings.TrimSpace(options.Endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("privacy/httpadapter: endpoint is invalid")
	}
	if parsed.Scheme != "https" &&
		!(parsed.Scheme == "http" && (isLoopback(parsed.Hostname()) || options.AllowInsecureHTTP)) {
		return nil, fmt.Errorf("privacy/httpadapter: endpoint must use HTTPS outside loopback")
	}
	client := options.Client
	if client == nil {
		client = http.DefaultClient
	}
	maxBytes := options.MaxBytes
	if maxBytes == 0 {
		maxBytes = 256 << 10
	}
	if maxBytes < 4096 || maxBytes > 4<<20 {
		return nil, fmt.Errorf("privacy/httpadapter: max bytes is out of range")
	}
	return &Client{endpoint: endpoint, client: client, tokenSource: options.TokenSource, maxBytes: maxBytes}, nil
}

func (client *Client) Handle(ctx context.Context, command privacy.OwnerCommand) (privacy.OwnerReceipt, error) {
	body, err := json.Marshal(command)
	if err != nil {
		return privacy.OwnerReceipt{}, fmt.Errorf("privacy/httpadapter: encode command: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(body))
	if err != nil {
		return privacy.OwnerReceipt{}, fmt.Errorf("privacy/httpadapter: build request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if client.tokenSource != nil {
		token, err := client.tokenSource(ctx)
		if err != nil {
			return privacy.OwnerReceipt{}, &privacy.Error{Kind: privacy.ErrorOwnerUnavailable, Field: "token", Message: "owner token is unavailable", Retryable: true, Cause: err}
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}
	response, err := client.client.Do(request)
	if err != nil {
		return privacy.OwnerReceipt{}, &privacy.Error{Kind: privacy.ErrorOwnerUnavailable, Field: "transport", Message: "owner request failed", Retryable: true, Cause: err}
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, client.maxBytes+1)
	encoded, err := io.ReadAll(limited)
	if err != nil {
		return privacy.OwnerReceipt{}, &privacy.Error{Kind: privacy.ErrorOwnerUnavailable, Field: "transport", Message: "owner response read failed", Retryable: true, Cause: err}
	}
	if int64(len(encoded)) > client.maxBytes {
		return privacy.OwnerReceipt{}, &privacy.Error{Kind: privacy.ErrorProtocolViolation, Field: "response", Message: "owner response exceeds limit"}
	}
	if response.StatusCode != http.StatusOK {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		kind := privacy.ErrorProtocolViolation
		if retryable {
			kind = privacy.ErrorOwnerUnavailable
		}
		return privacy.OwnerReceipt{}, &privacy.Error{
			Kind: kind, Field: "owner", Message: fmt.Sprintf("owner returned HTTP %d", response.StatusCode), Retryable: retryable,
		}
	}
	var receipt privacy.OwnerReceipt
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return privacy.OwnerReceipt{}, &privacy.Error{Kind: privacy.ErrorProtocolViolation, Field: "response", Message: "owner response is invalid", Cause: err}
	}
	return receipt, nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
