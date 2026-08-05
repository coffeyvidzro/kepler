package audit

import "context"

type RequestMetadata struct {
	RequestID string
	IPAddress string
	UserAgent string
}

type requestMetadataKey struct{}

func ContextWithRequestMetadata(ctx context.Context, metadata RequestMetadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestMetadataKey{}, metadata)
}

func RequestMetadataFromContext(ctx context.Context) (RequestMetadata, bool) {
	if ctx == nil {
		return RequestMetadata{}, false
	}
	metadata, ok := ctx.Value(requestMetadataKey{}).(RequestMetadata)
	return metadata, ok
}
