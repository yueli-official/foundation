package turnstile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/yueli-official/foundation/go/abuse"
)

const (
	DefaultEndpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	MaxTokenBytes   = 2048
)

type Options struct {
	Secret      string
	Endpoint    string
	HTTPClient  *http.Client
	MaxAttempts int
}

type Verifier struct {
	secret      string
	endpoint    string
	client      *http.Client
	maxAttempts int
}

func New(options Options) (*Verifier, error) {
	secret := strings.TrimSpace(options.Secret)
	if secret == "" {
		return nil, &abuse.Error{
			Kind: abuse.ErrorVerifierConfiguration, Field: "turnstile",
			Message: "secret is required",
		}
	}
	endpoint := strings.TrimSpace(options.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, &abuse.Error{
			Kind: abuse.ErrorVerifierConfiguration, Field: "turnstile",
			Message: "endpoint is invalid", Cause: err,
		}
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	attempts := options.MaxAttempts
	if attempts == 0 {
		attempts = 2
	}
	if attempts < 1 || attempts > 3 {
		return nil, &abuse.Error{
			Kind: abuse.ErrorVerifierConfiguration, Field: "turnstile",
			Message: "max attempts must be between 1 and 3",
		}
	}
	return &Verifier{secret: secret, endpoint: endpoint, client: client, maxAttempts: attempts}, nil
}

type siteverifyResponse struct {
	Success     bool       `json:"success"`
	ChallengeAt *time.Time `json:"challenge_ts"`
	Hostname    string     `json:"hostname"`
	Action      string     `json:"action"`
	ErrorCodes  []string   `json:"error-codes"`
}

func (verifier *Verifier) Verify(ctx context.Context, request abuse.VerificationRequest) (abuse.Verification, error) {
	if verifier == nil {
		return abuse.Verification{}, configuration("verifier is not initialized", nil)
	}
	if len(request.Token) == 0 || len(request.Token) > MaxTokenBytes {
		return abuse.Verification{Status: abuse.VerificationRejected}, nil
	}
	if strings.TrimSpace(request.VerificationID) == "" {
		return abuse.Verification{}, configuration("verification id is required", nil)
	}
	form := url.Values{
		"secret":          {verifier.secret},
		"response":        {request.Token},
		"idempotency_key": {request.VerificationID},
	}
	var last error
	for attempt := 1; attempt <= verifier.maxAttempts; attempt++ {
		response, retry, err := verifier.call(ctx, form)
		if err != nil {
			last = err
			if retry && attempt < verifier.maxAttempts && ctx.Err() == nil {
				continue
			}
			return abuse.Verification{}, err
		}
		if !response.Success {
			if slices.Contains(response.ErrorCodes, "invalid-input-secret") ||
				slices.Contains(response.ErrorCodes, "missing-input-secret") {
				return abuse.Verification{}, configuration("provider rejected verifier configuration", nil)
			}
			if slices.Contains(response.ErrorCodes, "internal-error") {
				last = unavailable("provider returned an internal error", nil)
				if attempt < verifier.maxAttempts && ctx.Err() == nil {
					continue
				}
				return abuse.Verification{}, last
			}
			return abuse.Verification{
				Status: abuse.VerificationRejected, Codes: sanitizeCodes(response.ErrorCodes),
			}, nil
		}
		if response.Action != request.ExpectedAction ||
			!slices.Contains(request.AllowedHosts, response.Hostname) {
			return abuse.Verification{
				Status: abuse.VerificationRejected, Action: response.Action,
				Hostname: response.Hostname,
			}, nil
		}
		verification := abuse.Verification{
			Status: abuse.VerificationAccepted, Action: response.Action,
			Hostname: response.Hostname, Codes: sanitizeCodes(response.ErrorCodes),
		}
		if response.ChallengeAt != nil {
			verification.SolvedAt = response.ChallengeAt.UTC()
		}
		return verification, nil
	}
	if last != nil {
		return abuse.Verification{}, last
	}
	return abuse.Verification{}, unavailable("verification did not complete", nil)
}

func (verifier *Verifier) call(ctx context.Context, form url.Values) (siteverifyResponse, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, verifier.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return siteverifyResponse{}, false, configuration("cannot create request", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := verifier.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return siteverifyResponse{}, false, ctx.Err()
		}
		return siteverifyResponse{}, true, unavailable("provider request failed", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return siteverifyResponse{}, response.StatusCode >= 500,
			unavailable(fmt.Sprintf("provider returned HTTP %d", response.StatusCode), nil)
	}
	var decoded siteverifyResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64<<10))
	if err := decoder.Decode(&decoded); err != nil {
		return siteverifyResponse{}, false, unavailable("provider returned malformed JSON", err)
	}
	return decoded, false, nil
}

func sanitizeCodes(codes []string) []string {
	result := make([]string, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code != "" && len(code) <= 100 {
			result = append(result, code)
		}
	}
	return result
}

func unavailable(message string, cause error) error {
	return &abuse.Error{
		Kind: abuse.ErrorVerifierUnavailable, Field: "turnstile",
		Message: message, Retryable: true, Cause: cause,
	}
}

func configuration(message string, cause error) error {
	return &abuse.Error{
		Kind: abuse.ErrorVerifierConfiguration, Field: "turnstile",
		Message: message, Cause: cause,
	}
}

var _ abuse.ChallengeVerifier = (*Verifier)(nil)
