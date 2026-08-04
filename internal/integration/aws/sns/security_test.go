package sns

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type securityRoundTripFunc func(*http.Request) (*http.Response, error)

func (f securityRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestHTTPCertificateLoaderCanonicalizesAllowlistedEndpoint(t *testing.T) {
	now := time.Date(2026, time.July, 31, 16, 0, 0, 0, time.UTC)
	certificatePEM := newSecurityTestCertificate(t, now)
	var requestedURL string
	client := &http.Client{Transport: securityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestedURL = request.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(certificatePEM)),
			Request:    request,
		}, nil
	})}
	loader := NewHTTPCertificateLoader(client)
	loader.now = func() time.Time { return now }

	_, err := loader.Load(context.Background(), "https://SNS.US-EAST-1.AMAZONAWS.COM/SimpleNotificationService-test.pem")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if requestedURL != "https://sns.us-east-1.amazonaws.com/SimpleNotificationService-test.pem" {
		t.Fatalf("request URL = %q", requestedURL)
	}
}

func TestHTTPCertificateLoaderRejectsUnknownRegionBeforeRequest(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: securityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("unexpected request")
	})}
	loader := NewHTTPCertificateLoader(client)

	_, err := loader.Load(context.Background(), "https://sns.internal-1.amazonaws.com/SimpleNotificationService-test.pem")
	if !errors.Is(err, ErrUntrustedCertificateURL) {
		t.Fatalf("Load() error = %v, want ErrUntrustedCertificateURL", err)
	}
	if requests != 0 {
		t.Fatalf("HTTP requests = %d, want 0", requests)
	}
}

func TestHTTPConfirmSubscriptionClientPostsFormToAllowlistedEndpoint(t *testing.T) {
	const topicARN = "arn:aws:sns:us-east-1:123456789012:ses-events"
	const token = "confirmation-token"
	var requestMethod string
	var requestURL string
	var contentType string
	var body string
	client := &http.Client{Transport: securityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestMethod = request.Method
		requestURL = request.URL.String()
		contentType = request.Header.Get("Content-Type")
		payload, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		body = string(payload)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("<ConfirmSubscriptionResponse/>")),
			Request:    request,
		}, nil
	})}

	err := NewHTTPConfirmSubscriptionClient(client).ConfirmSubscription(context.Background(), ConfirmSubscriptionInput{
		TopicARN: topicARN,
		Token:    token,
	})
	if err != nil {
		t.Fatalf("ConfirmSubscription() error = %v", err)
	}
	if requestMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", requestMethod)
	}
	if requestURL != "https://sns.us-east-1.amazonaws.com/" {
		t.Fatalf("request URL = %q", requestURL)
	}
	if contentType != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	values, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	if values.Get("Action") != "ConfirmSubscription" || values.Get("TopicArn") != topicARN || values.Get("Token") != token || values.Get("Version") != "2010-03-31" {
		t.Fatalf("form values = %#v", values)
	}
}

func TestHTTPConfirmSubscriptionClientRejectsUnknownRegionBeforeRequest(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: securityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("unexpected request")
	})}

	err := NewHTTPConfirmSubscriptionClient(client).ConfirmSubscription(context.Background(), ConfirmSubscriptionInput{
		TopicARN: "arn:aws:sns:internal-1:123456789012:ses-events",
		Token:    "confirmation-token",
	})
	if !errors.Is(err, ErrTopicNotAllowed) {
		t.Fatalf("ConfirmSubscription() error = %v, want ErrTopicNotAllowed", err)
	}
	if requests != 0 {
		t.Fatalf("HTTP requests = %d, want 0", requests)
	}
}

func TestConfirmationEndpointSupportsChinaPartition(t *testing.T) {
	endpoint, err := confirmationEndpoint(ConfirmSubscriptionInput{
		TopicARN: "arn:aws-cn:sns:cn-north-1:123456789012:ses-events",
		Token:    "confirmation-token",
	})
	if err != nil {
		t.Fatalf("confirmationEndpoint() error = %v", err)
	}
	if endpoint != "https://sns.cn-north-1.amazonaws.com.cn/" {
		t.Fatalf("endpoint = %q", endpoint)
	}
}

func newSecurityTestCertificate(t *testing.T, now time.Time) []byte {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "sns.test"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
