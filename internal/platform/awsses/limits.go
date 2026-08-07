package awsses

const (
	// MaxRawMessageBytes is the authoritative provider limit for the fully
	// encoded RFC 5322 message submitted through SES v2 SendEmail.
	MaxRawMessageBytes = 40 << 20

	// MaxBodyBytes applies independently to the HTML and text alternatives.
	MaxBodyBytes = 1 << 20

	// MaxAttachmentsEncodedBytes matches Resend's aggregate attachment limit.
	// The limit is measured after Base64 encoding, before MIME line wrapping.
	MaxAttachmentsEncodedBytes = 40 << 20

	// MaxBatchPayloadBytes limits the sum of body and metadata bytes accepted by
	// one batch operation. Batch attachments are intentionally unsupported.
	MaxBatchPayloadBytes = 10 << 20

	// MaxHTTPRequestBytes accommodates a 40 MiB Base64 attachment payload plus
	// JSON fields and modest request overhead.
	MaxHTTPRequestBytes = 48 << 20
)
