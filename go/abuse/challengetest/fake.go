package challengetest

import (
	"context"
	"sync"

	"github.com/yueli-official/foundation/go/abuse"
)

// Verifier is a concurrency-safe programmable Challenge Adapter for consumer
// and conformance tests.
type Verifier struct {
	mu         sync.Mutex
	requests   []abuse.VerificationRequest
	VerifyFunc func(context.Context, abuse.VerificationRequest) (abuse.Verification, error)
}

func (verifier *Verifier) Verify(ctx context.Context, request abuse.VerificationRequest) (abuse.Verification, error) {
	verifier.mu.Lock()
	verifier.requests = append(verifier.requests, request)
	function := verifier.VerifyFunc
	verifier.mu.Unlock()
	if function == nil {
		return abuse.Verification{Status: abuse.VerificationRejected}, nil
	}
	return function(ctx, request)
}

func (verifier *Verifier) Requests() []abuse.VerificationRequest {
	verifier.mu.Lock()
	defer verifier.mu.Unlock()
	return append([]abuse.VerificationRequest(nil), verifier.requests...)
}

var _ abuse.ChallengeVerifier = (*Verifier)(nil)
