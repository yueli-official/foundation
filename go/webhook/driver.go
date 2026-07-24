package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/yueli-official/foundation/go/work"
)

const WorkKind work.Kind = "webhook.deliver"

func WorkDefinition(queue work.Queue) work.KindDefinition {
	return work.KindDefinition{
		Key: WorkKind, Queue: queue, DefaultAttempts: 100, MaxAttempts: 1000,
		Timeout: 45 * time.Second,
	}
}

type DeliveryDriver struct {
	Backend    DeliveryBackend
	Secrets    SecretStore
	Authorizer NetworkAuthorizer
	Sender     Sender
	Retry      RetryPolicy
	Limits     Limits
	Clock      func() time.Time
}

func (driver *DeliveryDriver) Advance(ctx context.Context, id DeliveryID) (DeliveryView, error) {
	if driver == nil || driver.Backend == nil || driver.Secrets == nil || driver.Sender == nil {
		return DeliveryView{}, invalid(ErrorInvalidDefinition, "driver", "backend, secrets and sender are required")
	}
	now := time.Now().UTC()
	if driver.Clock != nil {
		now = driver.Clock().UTC()
	}
	plan, err := driver.Backend.BeginAttempt(ctx, id)
	if err != nil {
		if IsCode(err, ErrorStateConflict) || IsCode(err, ErrorNotFound) {
			return DeliveryView{}, work.Permanent(err)
		}
		return DeliveryView{}, err
	}
	completion := AttemptCompletion{
		Plan: plan, Outcome: AttemptRetryable, ErrorCode: "internal",
		FinishedAt: now,
	}
	secrets, err := driver.Secrets.Resolve(ctx, plan.Secret, now)
	if err != nil {
		completion.ErrorCode = "secret"
		driver.prepareRetry(&completion, now, 0)
		delivery, completeErr := driver.Backend.CompleteAttempt(ctx, completion)
		if completeErr != nil {
			return DeliveryView{}, errors.Join(err, completeErr)
		}
		if completion.Outcome == AttemptPermanent {
			return delivery, work.Permanent(err)
		}
		return delivery, work.RetryAfter(err, completion.NextAttemptAt.Sub(now))
	}
	route, err := driver.Authorizer.Authorize(ctx, plan.URL)
	if err != nil {
		if retryableError(err) {
			completion.ErrorCode = "dns"
			driver.prepareRetry(&completion, now, 0)
		} else {
			completion.ErrorCode = "security"
			completion.Outcome = AttemptPermanent
		}
		delivery, completeErr := driver.Backend.CompleteAttempt(ctx, completion)
		if completeErr != nil {
			return DeliveryView{}, errors.Join(err, completeErr)
		}
		if completion.Outcome == AttemptRetryable {
			return delivery, work.RetryAfter(err, completion.NextAttemptAt.Sub(now))
		}
		return delivery, work.Permanent(err)
	}
	headers := make(http.Header)
	headers.Set("Content-Type", CloudEventsContentType)
	headers.Set(HeaderWebhookID, string(plan.EventID))
	headers.Set(HeaderWebhookTimestamp, strconv.FormatInt(now.Unix(), 10))
	headers.Set(HeaderDeliveryID, string(plan.DeliveryID))
	headers.Add(HeaderWebhookSignature, SignV1(string(plan.EventID), now, plan.Body, secrets.Primary.Value))
	for _, previous := range secrets.Previous {
		headers.Add(HeaderWebhookSignature, SignV1(string(plan.EventID), now, plan.Body, previous.Value))
	}
	result, sendErr := driver.Sender.Send(ctx, OutboundRequest{
		Route: route, Header: headers, Body: plan.Body,
		Limits: ExchangeLimits{
			Timeout:          driver.Retry.RequestTimeout,
			MaxResponseBytes: int64(driver.Limits.MaxResponseBytes),
			MaxHeaderBytes:   64 << 10,
		},
	})
	completion.FinishedAt = result.FinishedAt
	if completion.FinishedAt.IsZero() {
		completion.FinishedAt = now
	}
	completion.StatusCode = result.StatusCode
	completion.ResponseDigest = result.ResponseDigest
	completion.SecretRevision = secrets.Primary.Revision
	switch {
	case sendErr != nil:
		if IsCode(sendErr, ErrorEndpointUnsafe) {
			completion.Outcome, completion.ErrorCode = AttemptPermanent, "security"
		} else {
			completion.Outcome, completion.ErrorCode = AttemptRetryable, "transport"
		}
	case result.StatusCode >= 200 && result.StatusCode <= 299:
		completion.Outcome, completion.ErrorCode = AttemptSucceeded, ""
	case result.StatusCode == http.StatusGone:
		completion.Outcome, completion.ErrorCode = AttemptPermanent, "http_410"
		completion.DisableEndpoint = true
	case result.StatusCode == http.StatusRequestTimeout ||
		result.StatusCode == http.StatusTooEarly ||
		result.StatusCode == http.StatusTooManyRequests ||
		result.StatusCode >= 500:
		completion.Outcome = AttemptRetryable
		completion.ErrorCode = fmt.Sprintf("http_%d", result.StatusCode)
	default:
		completion.Outcome = AttemptPermanent
		completion.ErrorCode = fmt.Sprintf("http_%d", result.StatusCode)
	}
	if completion.Outcome == AttemptRetryable {
		driver.prepareRetry(&completion, now, result.RetryAfter)
	}
	delivery, completeErr := driver.Backend.CompleteAttempt(ctx, completion)
	if completeErr != nil {
		return DeliveryView{}, completeErr
	}
	switch completion.Outcome {
	case AttemptSucceeded:
		return delivery, nil
	case AttemptRetryable:
		return delivery, work.RetryAfter(
			&Error{Code: ErrorUnavailable, Field: "delivery", Message: completion.ErrorCode, Retryable: true, Cause: sendErr},
			completion.NextAttemptAt.Sub(now),
		)
	default:
		return delivery, work.Permanent(
			&Error{Code: ErrorUnavailable, Field: "delivery", Message: completion.ErrorCode, Cause: sendErr},
		)
	}
}

func (driver *DeliveryDriver) prepareRetry(completion *AttemptCompletion, now time.Time, retryAfter time.Duration) {
	completion.NextAttemptAt = driver.nextAttempt(completion.Plan, now, retryAfter)
	if completion.Plan.Number >= driver.Retry.MaxAttempts ||
		completion.NextAttemptAt.After(completion.Plan.DeliveryCreated.Add(driver.Retry.MaxAge)) {
		completion.Outcome = AttemptPermanent
		completion.ErrorCode = "retry_exhausted"
		completion.NextAttemptAt = time.Time{}
	}
}

func retryableError(err error) bool {
	var typed *Error
	return errors.As(err, &typed) && typed.Retryable
}

func (driver *DeliveryDriver) nextAttempt(plan AttemptPlan, now time.Time, retryAfter time.Duration) time.Time {
	delay := driver.Retry.BaseDelay
	for index := 1; index < plan.Number && delay < driver.Retry.MaxDelay; index++ {
		delay *= 2
		if delay > driver.Retry.MaxDelay {
			delay = driver.Retry.MaxDelay
		}
	}
	if retryAfter > 0 {
		if retryAfter > driver.Retry.MaxRetryAfter {
			retryAfter = driver.Retry.MaxRetryAfter
		}
		if retryAfter > delay {
			delay = retryAfter
		}
	}
	return now.Add(delay)
}

func NewWorkHandler(driver *DeliveryDriver) work.Handler {
	return work.HandlerFunc(func(ctx context.Context, job work.Job, _ work.Progress) (work.Result, error) {
		var payload DeliveryWork
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return work.Result{}, work.Permanent(fmt.Errorf("webhook: decode delivery work: %w", err))
		}
		delivery, err := driver.Advance(ctx, payload.DeliveryID)
		if err != nil {
			return work.Result{}, err
		}
		return work.Result{Summary: string(delivery.State)}, nil
	})
}
