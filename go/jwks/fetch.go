package jwks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	jose "github.com/go-jose/go-jose/v4"
)

func fetch(ctx context.Context, client *http.Client, endpoint string, maxBodyBytes int64) (map[string]any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("jwks: create request: %w", err)
	}
	request.Header.Set("Accept", "application/jwk-set+json, application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("jwks: fetch: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks: fetch: unexpected HTTP status %d", response.StatusCode)
	}

	limited := io.LimitReader(response.Body, maxBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("jwks: read response: %w", err)
	}
	if int64(len(body)) > maxBodyBytes {
		return nil, ErrBodyTooLarge
	}
	var set jose.JSONWebKeySet
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, fmt.Errorf("jwks: decode response: %w", err)
	}
	if len(set.Keys) == 0 {
		return nil, ErrNoUsableKeys
	}
	keys, err := indexKeys(set)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, errors.New("jwks: decoded response has no usable keys")
	}
	return keys, nil
}
