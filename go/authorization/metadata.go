package authorization

import "context"

// RequestMetadata is transport-neutral trace context for commands. Consumers
// bind it from trusted request/event metadata, never from an actor field in a
// JSON body.
type RequestMetadata struct {
	CorrelationID string
}

type requestMetadataKey struct{}

func WithRequestMetadata(ctx context.Context, metadata RequestMetadata) context.Context {
	return context.WithValue(ctx, requestMetadataKey{}, metadata)
}

func RequestMetadataFromContext(ctx context.Context) RequestMetadata {
	if ctx == nil {
		return RequestMetadata{}
	}
	metadata, _ := ctx.Value(requestMetadataKey{}).(RequestMetadata)
	return metadata
}
