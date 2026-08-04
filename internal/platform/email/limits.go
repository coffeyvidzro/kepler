package email

const (
	// MaxRawMessageBytes is the authoritative provider limit for the fully
	// encoded RFC 5322 message submitted through SES SendRawEmail.
	MaxRawMessageBytes = 10 << 20

	// MaxBodyBytes applies independently to the HTML and text alternatives.
	MaxBodyBytes = 1 << 20

	// MaxAttachmentsDecodedBytes is the aggregate decoded attachment allowance.
	// Base64 line wrapping expands this to roughly 9.6 MiB, leaving space for
	// headers, MIME boundaries, and message bodies under MaxRawMessageBytes.
	MaxAttachmentsDecodedBytes = 7 << 20

	// MaxBatchPayloadBytes limits the sum of body and metadata bytes accepted by
	// one batch operation. Batch attachments are intentionally unsupported.
	MaxBatchPayloadBytes = 10 << 20

	// MaxHTTPRequestBytes accommodates the JSON and base64 representation while
	// remaining close to the provider's final encoded-message limit.
	MaxHTTPRequestBytes = 12 << 20
)
