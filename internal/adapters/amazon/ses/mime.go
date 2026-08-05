package ses

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"path/filepath"
	"strings"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

func buildMIME(message platformemail.Message) ([]byte, error) {
	if strings.TrimSpace(message.From.Email) == "" || len(message.To)+len(message.CC)+len(message.BCC) == 0 {
		return nil, errors.New("email requires a sender and at least one recipient")
	}
	if message.Text == "" && message.HTML == "" {
		return nil, errors.New("email requires a text or HTML body")
	}
	if err := validateCustomHeaders(message.Headers); err != nil {
		return nil, err
	}

	var output bytes.Buffer
	writeHeader(&output, "From", formatAddress(message.From))
	writeHeader(&output, "To", joinAddresses(message.To))
	writeHeader(&output, "Cc", joinAddresses(message.CC))
	writeHeader(&output, "Reply-To", joinAddresses(message.ReplyTo))
	writeHeader(&output, "Subject", mime.QEncoding.Encode("UTF-8", message.Subject))
	writeHeader(&output, "MIME-Version", "1.0")
	for key, value := range message.Headers {
		writeHeader(&output, textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(key)), value)
	}
	mixed := multipart.NewWriter(&output)
	writeHeader(&output, "Content-Type", fmt.Sprintf("multipart/mixed; boundary=%q", mixed.Boundary()))
	output.WriteString("\r\n")
	bodyHeader := textproto.MIMEHeader{}
	bodyBoundary := randomBoundary()
	bodyHeader.Set("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", bodyBoundary))
	bodyPart, err := mixed.CreatePart(bodyHeader)
	if err != nil {
		return nil, fmt.Errorf("create email body: %w", err)
	}
	alternative := multipart.NewWriter(bodyPart)
	if err := alternative.SetBoundary(bodyBoundary); err != nil {
		return nil, fmt.Errorf("set email body boundary: %w", err)
	}
	if message.Text != "" {
		if err := writeBodyPart(alternative, "text/plain; charset=UTF-8", message.Text); err != nil {
			return nil, err
		}
	}
	if message.HTML != "" {
		if err := writeBodyPart(alternative, "text/html; charset=UTF-8", message.HTML); err != nil {
			return nil, err
		}
	}
	if err := alternative.Close(); err != nil {
		return nil, fmt.Errorf("close email body: %w", err)
	}
	for _, attachment := range message.Attachments {
		if strings.TrimSpace(attachment.Path) != "" && strings.TrimSpace(attachment.Content) == "" {
			return nil, ErrUnsupportedAttachmentPath
		}
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(attachment.Content))
		if err != nil {
			return nil, fmt.Errorf("decode attachment %q: %w", attachment.Filename, err)
		}
		filename := filepath.Base(strings.TrimSpace(attachment.Filename))
		if filename == "" || filename == "." {
			return nil, errors.New("attachment filename is required")
		}
		contentType := sanitizeHeaderValue(attachment.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		if _, _, err := mime.ParseMediaType(contentType); err != nil {
			return nil, fmt.Errorf("invalid attachment content type %q: %w", contentType, err)
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Type", contentType)
		header.Set("Content-Transfer-Encoding", "base64")
		header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
		if id := sanitizeHeaderValue(attachment.ContentID); id != "" {
			header.Set("Content-ID", "<"+id+">")
		}
		part, err := mixed.CreatePart(header)
		if err != nil {
			return nil, fmt.Errorf("create attachment %q: %w", filename, err)
		}
		if err := writeBase64Lines(part, data); err != nil {
			return nil, fmt.Errorf("write attachment %q: %w", filename, err)
		}
	}
	if err := mixed.Close(); err != nil {
		return nil, fmt.Errorf("close MIME message: %w", err)
	}
	if output.Len() > platformemail.MaxRawMessageBytes {
		return nil, fmt.Errorf("%w: encoded message is %d bytes; maximum is %d bytes", ErrMessageTooLarge, output.Len(), platformemail.MaxRawMessageBytes)
	}
	return output.Bytes(), nil
}

func validateCustomHeaders(headers map[string]string) error {
	for key, value := range headers {
		key = textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(key))
		if key == "" || strings.ContainsAny(value, "\r\n") {
			return errors.New("email headers must not be empty or contain newlines")
		}
		if reservedHeader(key) {
			return fmt.Errorf("%w: %s", ErrReservedHeader, key)
		}
	}
	return nil
}

func writeBase64Lines(part interface{ Write([]byte) (int, error) }, data []byte) error {
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 76 {
		if _, err := part.Write([]byte(encoded[:76] + "\r\n")); err != nil {
			return err
		}
		encoded = encoded[76:]
	}
	if encoded != "" {
		_, err := part.Write([]byte(encoded + "\r\n"))
		return err
	}
	return nil
}

func writeBodyPart(writer *multipart.Writer, contentType, body string) error {
	header := textproto.MIMEHeader{}
	header.Set("Content-Type", contentType)
	header.Set("Content-Transfer-Encoding", "quoted-printable")
	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("create MIME body part: %w", err)
	}
	encoded := quotedprintable.NewWriter(part)
	if _, err := encoded.Write([]byte(body)); err != nil {
		return fmt.Errorf("write MIME body part: %w", err)
	}
	if err := encoded.Close(); err != nil {
		return fmt.Errorf("close MIME body part: %w", err)
	}
	return nil
}

func formatAddress(address platformemail.Address) string {
	return (&mail.Address{Name: strings.TrimSpace(address.Name), Address: strings.TrimSpace(address.Email)}).String()
}

func joinAddresses(addresses []platformemail.Address) string {
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, formatAddress(address))
	}
	return strings.Join(values, ", ")
}

func writeHeader(output *bytes.Buffer, key, value string) {
	value = sanitizeHeaderValue(value)
	if key != "" && value != "" {
		fmt.Fprintf(output, "%s: %s\r\n", key, value)
	}
}

func sanitizeHeaderValue(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(value), "\r", ""), "\n", "")
}

func reservedHeader(key string) bool {
	canonical := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(key))
	lower := strings.ToLower(canonical)
	if strings.HasPrefix(lower, "x-ses-") || strings.HasPrefix(lower, "x-amazon-") {
		return true
	}
	switch canonical {
	case "From", "To", "Cc", "Bcc", "Reply-To", "Sender", "Subject", "Date", "Message-Id", "Return-Path", "MIME-Version", "Content-Type", "Content-Transfer-Encoding":
		return true
	default:
		return false
	}
}

func randomBoundary() string {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	boundary := writer.Boundary()
	_ = writer.Close()
	return boundary
}
