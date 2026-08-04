package sns

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"
)

const testCertificateURL = "https://sns.us-east-1.amazonaws.com/SimpleNotificationService-test.pem"

type staticCertificateLoader struct {
	certificate *x509.Certificate
	err         error
	calls       int
}

func (l *staticCertificateLoader) Load(context.Context, string) (*x509.Certificate, error) {
	l.calls++
	return l.certificate, l.err
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestParseEnvelopePreservesTimestamp(t *testing.T) {
	subject := "Test subject"
	raw := []byte(`{
		"Type":"Notification",
		"MessageId":"message-id",
		"TopicArn":"arn:aws:sns:us-east-1:123456789012:ses-events",
		"Subject":"Test subject",
		"Message":"hello",
		"Timestamp":"2026-07-31T07:49:00.1200Z",
		"SignatureVersion":"2",
		"Signature":"c2lnbmF0dXJl",
		"SigningCertURL":"https://sns.us-east-1.amazonaws.com/SimpleNotificationService-test.pem"
	}`)

	envelope, err := ParseEnvelope(raw)
	if err != nil {
		t.Fatalf("ParseEnvelope() error = %v", err)
	}
	if envelope.Timestamp != "2026-07-31T07:49:00.1200Z" {
		t.Fatalf("Timestamp = %q", envelope.Timestamp)
	}
	if envelope.Subject == nil || *envelope.Subject != subject {
		t.Fatalf("Subject = %#v", envelope.Subject)
	}
}

func TestVerifierAcceptsNotificationVersion2(t *testing.T) {
	now := time.Date(2026, 7, 31, 7, 49, 0, 0, time.UTC)
	privateKey, certificate, _ := newTestCertificate(t, now)
	subject := "Delivery event"
	envelope := Envelope{
		Type: TypeNotification, MessageID: "message-id", TopicARN: "arn:aws:sns:us-east-1:123456789012:ses-events",
		Subject: &subject, Message: `{"eventType":"Delivery"}`, Timestamp: now.Format(time.RFC3339Nano),
		SignatureVersion: "2", SigningCertURL: testCertificateURL,
	}
	signEnvelope(t, privateKey, &envelope)
	loader := &staticCertificateLoader{certificate: certificate}
	verifier := NewVerifier([]string{envelope.TopicARN}, loader)

	if err := verifier.Verify(context.Background(), envelope); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if loader.calls != 1 {
		t.Fatalf("certificate loader calls = %d, want 1", loader.calls)
	}
}

func TestVerifierAcceptsSubscriptionConfirmationVersion1(t *testing.T) {
	now := time.Date(2026, 7, 31, 7, 49, 0, 0, time.UTC)
	privateKey, certificate, _ := newTestCertificate(t, now)
	subscribeURL := "https://sns.us-east-1.amazonaws.com/?Action=ConfirmSubscription"
	token := "confirmation-token"
	envelope := Envelope{
		Type: TypeSubscriptionConfirmation, MessageID: "message-id", TopicARN: "arn:aws:sns:us-east-1:123456789012:ses-events",
		Message: "confirm subscription", Timestamp: now.Format(time.RFC3339Nano), SignatureVersion: "1",
		SigningCertURL: testCertificateURL, SubscribeURL: &subscribeURL, Token: &token,
	}
	signEnvelope(t, privateKey, &envelope)
	verifier := NewVerifier([]string{envelope.TopicARN}, &staticCertificateLoader{certificate: certificate})

	if err := verifier.Verify(context.Background(), envelope); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifierRejectsModifiedMessage(t *testing.T) {
	now := time.Date(2026, 7, 31, 7, 49, 0, 0, time.UTC)
	privateKey, certificate, _ := newTestCertificate(t, now)
	envelope := Envelope{
		Type: TypeNotification, MessageID: "message-id", TopicARN: "arn:aws:sns:us-east-1:123456789012:ses-events",
		Message: "original", Timestamp: now.Format(time.RFC3339Nano), SignatureVersion: "2", SigningCertURL: testCertificateURL,
	}
	signEnvelope(t, privateKey, &envelope)
	envelope.Message = "modified"
	verifier := NewVerifier([]string{envelope.TopicARN}, &staticCertificateLoader{certificate: certificate})

	err := verifier.Verify(context.Background(), envelope)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("Verify() error = %v, want ErrInvalidSignature", err)
	}
}

func TestVerifierRejectsDisallowedTopicBeforeLoadingCertificate(t *testing.T) {
	now := time.Date(2026, 7, 31, 7, 49, 0, 0, time.UTC)
	privateKey, certificate, _ := newTestCertificate(t, now)
	envelope := Envelope{
		Type: TypeNotification, MessageID: "message-id", TopicARN: "arn:aws:sns:us-east-1:123456789012:other-topic",
		Message: "event", Timestamp: now.Format(time.RFC3339Nano), SignatureVersion: "2", SigningCertURL: testCertificateURL,
	}
	signEnvelope(t, privateKey, &envelope)
	loader := &staticCertificateLoader{certificate: certificate}
	verifier := NewVerifier([]string{"arn:aws:sns:us-east-1:123456789012:ses-events"}, loader)

	err := verifier.Verify(context.Background(), envelope)
	if !errors.Is(err, ErrTopicNotAllowed) {
		t.Fatalf("Verify() error = %v, want ErrTopicNotAllowed", err)
	}
	if loader.calls != 0 {
		t.Fatalf("certificate loader calls = %d, want 0", loader.calls)
	}
}

func TestValidateSigningCertificateURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "regional", value: testCertificateURL},
		{name: "china", value: "https://sns.cn-north-1.amazonaws.com.cn/SimpleNotificationService-test.pem"},
		{name: "http", value: "http://sns.us-east-1.amazonaws.com/SimpleNotificationService-test.pem", wantErr: true},
		{name: "untrusted host", value: "https://example.com/SimpleNotificationService-test.pem", wantErr: true},
		{name: "suffix confusion", value: "https://sns.us-east-1.amazonaws.com.example.com/SimpleNotificationService-test.pem", wantErr: true},
		{name: "port", value: "https://sns.us-east-1.amazonaws.com:8443/SimpleNotificationService-test.pem", wantErr: true},
		{name: "nested path", value: "https://sns.us-east-1.amazonaws.com/certs/SimpleNotificationService-test.pem", wantErr: true},
		{name: "query", value: testCertificateURL + "?redirect=https://example.com", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSigningCertificateURL(test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateSigningCertificateURL() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestHTTPCertificateLoaderLoadsAndCachesCertificate(t *testing.T) {
	now := time.Date(2026, 7, 31, 7, 49, 0, 0, time.UTC)
	_, certificate, certificatePEM := newTestCertificate(t, now)
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(certificatePEM)),
			Request:    request,
		}, nil
	})}
	loader := NewHTTPCertificateLoader(client)
	loader.now = func() time.Time { return now }

	first, err := loader.Load(context.Background(), testCertificateURL)
	if err != nil {
		t.Fatalf("Load() first error = %v", err)
	}
	second, err := loader.Load(context.Background(), testCertificateURL)
	if err != nil {
		t.Fatalf("Load() second error = %v", err)
	}
	if !first.Equal(certificate) || !second.Equal(certificate) {
		t.Fatal("Load() returned an unexpected certificate")
	}
	if requests != 1 {
		t.Fatalf("HTTP requests = %d, want 1", requests)
	}
}

func TestCanonicalMessageHasNoTrailingNewline(t *testing.T) {
	envelope := Envelope{
		Type: TypeNotification, MessageID: "message-id", TopicARN: "arn:aws:sns:us-east-1:123456789012:ses-events",
		Message: "event", Timestamp: "2026-07-31T07:49:00Z", SignatureVersion: "2", Signature: "signature", SigningCertURL: testCertificateURL,
	}
	canonical, err := canonicalMessage(envelope)
	if err != nil {
		t.Fatalf("canonicalMessage() error = %v", err)
	}
	if strings.HasSuffix(string(canonical), "\n") {
		t.Fatalf("canonical message has a trailing newline: %q", canonical)
	}
}

func newTestCertificate(t *testing.T, now time.Time) (*rsa.PrivateKey, *x509.Certificate, []byte) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "sns.test"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate() error = %v", err)
	}
	return privateKey, certificate, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func signEnvelope(t *testing.T, privateKey *rsa.PrivateKey, envelope *Envelope) {
	t.Helper()
	envelope.Signature = "pending"
	canonical, err := canonicalMessage(*envelope)
	if err != nil {
		t.Fatalf("canonicalMessage() error = %v", err)
	}
	hash, err := signatureHash(envelope.SignatureVersion)
	if err != nil {
		t.Fatalf("signatureHash() error = %v", err)
	}
	digest := hash.New()
	_, _ = digest.Write(canonical)
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, hash, digest.Sum(nil))
	if err != nil {
		t.Fatalf("rsa.SignPKCS1v15() error = %v", err)
	}
	envelope.Signature = base64.StdEncoding.EncodeToString(signature)
}

func TestEnvelopeJSONRoundTrip(t *testing.T) {
	envelope := Envelope{Type: TypeNotification, MessageID: "id", TopicARN: "arn", Message: "message", Timestamp: "2026-07-31T07:49:00Z", SignatureVersion: "2", Signature: "signature", SigningCertURL: testCertificateURL}
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	parsed, err := ParseEnvelope(body)
	if err != nil {
		t.Fatalf("ParseEnvelope() error = %v", err)
	}
	if parsed.MessageID != envelope.MessageID {
		t.Fatalf("MessageID = %q, want %q", parsed.MessageID, envelope.MessageID)
	}
}
