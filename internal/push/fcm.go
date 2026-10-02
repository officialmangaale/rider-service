// Package push sends rider device notifications through Firebase Cloud
// Messaging (HTTP v1).
//
// It exists because a delivery offer used to reach a rider only over the rider's
// WebSocket and the app's own polling. A phone with the app in the background,
// or with the app process gone, has neither, so a rider who was eligible and
// nearby simply never heard about the order. The offer row is still the source
// of truth (see DeliveryService.sendOffers); a push is a hint that wakes the
// app so it can alert the rider, never the offer itself.
//
// The client is stdlib-only on purpose: FCM v1 needs an OAuth2 access token
// minted from a service-account key, which is a signed JWT and one form POST,
// and pulling in the Firebase Admin SDK for that would add a large dependency
// tree to a service that otherwise has none of it.
package push

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultFCMEndpoint = "https://fcm.googleapis.com"
	defaultTokenURI    = "https://oauth2.googleapis.com/token"
	fcmScope           = "https://www.googleapis.com/auth/firebase.messaging"

	// maxResponseBytes bounds what is read from Google's endpoints.
	maxResponseBytes = 1 << 20
)

// ErrUnregistered means FCM will never deliver to this token again (the app was
// uninstalled, its data cleared, or the token replaced). The caller should
// forget the token.
var ErrUnregistered = errors.New("push: device token is no longer registered")

// SendError is a failed send that is not an unregistered token.
type SendError struct {
	// Status is the HTTP status of the FCM response, 0 for a transport error.
	Status int
	// Code is FCM's error code (for example QUOTA_EXCEEDED), when it sent one.
	Code    string
	Message string
	// Retryable reports whether trying again later can help.
	Retryable bool
}

func (e *SendError) Error() string {
	return fmt.Sprintf("push: fcm send failed status=%d code=%s: %s", e.Status, e.Code, e.Message)
}

// ServiceAccount is the part of a Google service-account key the client needs.
type ServiceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// ParseServiceAccount reads a service-account key from raw JSON or from the
// same JSON base64-encoded (convenient for a single-line environment value).
func ParseServiceAccount(value string) (*ServiceAccount, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("push: empty service account")
	}
	raw := []byte(value)
	if !strings.HasPrefix(value, "{") {
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, errors.New("push: service account is neither JSON nor base64 JSON")
		}
		raw = decoded
	}
	var account ServiceAccount
	if err := json.Unmarshal(raw, &account); err != nil {
		return nil, fmt.Errorf("push: service account JSON: %w", err)
	}
	if account.ClientEmail == "" || account.PrivateKey == "" {
		return nil, errors.New("push: service account is missing client_email or private_key")
	}
	return &account, nil
}

// Alert is the visible text of an iOS notification. Android delivery is
// data-only, so the app builds its own notification (with Accept and Decline
// actions), which FCM cannot do on its behalf.
type Alert struct {
	Title string
	Body  string
}

// Message is one push to one device token.
type Message struct {
	Token string

	// Data is delivered to the app as-is. FCM requires every value to be a
	// string.
	Data map[string]string

	// TTL is how long FCM may hold the message for an unreachable device. A
	// delivery offer is worthless once it expires, so this is the offer's
	// remaining life: a phone that reconnects after that never sees it.
	TTL time.Duration

	// CollapseKey lets a newer message for the same subject replace an older
	// one that has not been delivered yet.
	CollapseKey string

	// Alert makes the iOS delivery a visible alert. Nil sends a silent
	// (content-available) message instead.
	Alert *Alert

	// APNSCategory names the iOS notification category that carries the
	// action buttons, when the app registers one.
	APNSCategory string
}

// Sender delivers one message. FCMClient is the production implementation.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Config configures an FCMClient.
type Config struct {
	Account ServiceAccount

	// ProjectID overrides the service account's project. The FCM project must
	// be the one the rider app's google-services.json belongs to.
	ProjectID string

	// Endpoint and HTTPClient exist for tests; the defaults are production.
	Endpoint   string
	HTTPClient *http.Client
	Now        func() time.Time
}

// FCMClient sends messages through FCM HTTP v1.
type FCMClient struct {
	projectID string
	account   ServiceAccount
	key       *rsa.PrivateKey
	tokenURI  string
	endpoint  string
	http      *http.Client
	now       func() time.Time

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// NewFCMClient validates the service-account key and returns a client. It does
// not contact Google: a bad key fails here, at start-up, not at the first offer.
func NewFCMClient(cfg Config) (*FCMClient, error) {
	key, err := parseRSAKey(cfg.Account.PrivateKey)
	if err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(cfg.ProjectID)
	if projectID == "" {
		projectID = strings.TrimSpace(cfg.Account.ProjectID)
	}
	if projectID == "" {
		return nil, errors.New("push: FCM project id is required (service account project_id or FCM_PROJECT_ID)")
	}
	client := &FCMClient{
		projectID: projectID,
		account:   cfg.Account,
		key:       key,
		tokenURI:  firstNonEmpty(cfg.Account.TokenURI, defaultTokenURI),
		endpoint:  strings.TrimRight(firstNonEmpty(cfg.Endpoint, defaultFCMEndpoint), "/"),
		http:      cfg.HTTPClient,
		now:       cfg.Now,
	}
	if client.http == nil {
		client.http = &http.Client{Timeout: 10 * time.Second}
	}
	if client.now == nil {
		client.now = time.Now
	}
	return client, nil
}

// ProjectID reports the FCM project messages are sent to.
func (c *FCMClient) ProjectID() string { return c.projectID }

// Send delivers msg. It returns ErrUnregistered for a token FCM has retired and
// a *SendError for any other failure.
func (c *FCMClient) Send(ctx context.Context, msg Message) error {
	if strings.TrimSpace(msg.Token) == "" {
		return &SendError{Code: "INVALID_ARGUMENT", Message: "empty device token"}
	}
	body, err := json.Marshal(map[string]any{"message": buildFCMMessage(msg, c.now())})
	if err != nil {
		return err
	}

	// One retry: after a 401 with a fresh access token (the cached one may have
	// been revoked), and after a transient failure.
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
		token, err := c.token(ctx, attempt > 0)
		if err != nil {
			return err
		}
		err = c.post(ctx, token, body)
		if err == nil || errors.Is(err, ErrUnregistered) {
			return err
		}
		last = err
		var sendErr *SendError
		if !errors.As(err, &sendErr) || !sendErr.Retryable {
			return err
		}
	}
	return last
}

func (c *FCMClient) post(ctx context.Context, accessToken string, body []byte) error {
	endpoint := fmt.Sprintf("%s/v1/projects/%s/messages:send", c.endpoint, url.PathEscape(c.projectID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return &SendError{Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return classifyFCMError(resp.StatusCode, payload)
}

// classifyFCMError maps an FCM error response. See
// https://firebase.google.com/docs/reference/fcm/rest/v1/ErrorCode.
func classifyFCMError(status int, payload []byte) error {
	var envelope struct {
		Error struct {
			Status  string `json:"status"`
			Message string `json:"message"`
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(payload, &envelope)
	code := envelope.Error.Status
	for _, detail := range envelope.Error.Details {
		if detail.ErrorCode != "" {
			code = detail.ErrorCode
			break
		}
	}
	message := envelope.Error.Message
	if message == "" {
		message = strings.TrimSpace(string(payload))
		if len(message) > 200 {
			message = message[:200]
		}
	}

	switch {
	case code == "UNREGISTERED", status == http.StatusNotFound:
		return ErrUnregistered
	case code == "INVALID_ARGUMENT" && strings.Contains(strings.ToLower(message), "registration token"):
		// A malformed token can never work either. Other INVALID_ARGUMENT
		// errors describe the payload, and forgetting tokens for those would
		// silently unsubscribe every rider.
		return ErrUnregistered
	}
	return &SendError{
		Status:  status,
		Code:    code,
		Message: message,
		// 401 is retried once with a fresh access token; 429 and 5xx are
		// transient.
		Retryable: status == http.StatusUnauthorized || status == http.StatusTooManyRequests || status >= 500,
	}
}

// buildFCMMessage is the v1 request body for msg.
func buildFCMMessage(msg Message, now time.Time) map[string]any {
	ttl := msg.TTL
	if ttl <= 0 {
		ttl = time.Minute
	}
	seconds := int64(ttl.Round(time.Second) / time.Second)
	if seconds < 1 {
		seconds = 1
	}

	android := map[string]any{
		// HIGH wakes a dozing device. It is honoured for messages that lead to
		// a user-visible notification, which is what an offer does.
		"priority": "HIGH",
		"ttl":      strconv.FormatInt(seconds, 10) + "s",
	}
	if msg.CollapseKey != "" {
		android["collapse_key"] = msg.CollapseKey
	}

	headers := map[string]string{
		"apns-expiration": strconv.FormatInt(now.Add(ttl).Unix(), 10),
	}
	aps := map[string]any{}
	if msg.Alert != nil {
		headers["apns-push-type"] = "alert"
		headers["apns-priority"] = "10"
		aps["alert"] = map[string]string{"title": msg.Alert.Title, "body": msg.Alert.Body}
		aps["sound"] = "default"
		aps["interruption-level"] = "time-sensitive"
		if msg.APNSCategory != "" {
			aps["category"] = msg.APNSCategory
		}
	} else {
		headers["apns-push-type"] = "background"
		headers["apns-priority"] = "5"
		aps["content-available"] = 1
	}
	if msg.CollapseKey != "" {
		headers["apns-collapse-id"] = msg.CollapseKey
	}

	return map[string]any{
		"token":   msg.Token,
		"data":    msg.Data,
		"android": android,
		"apns":    map[string]any{"headers": headers, "payload": map[string]any{"aps": aps}},
	}
}

// token returns a cached OAuth2 access token, minting a new one when it is
// missing, about to expire, or force is set.
func (c *FCMClient) token(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.accessToken != "" && c.now().Before(c.expiresAt.Add(-time.Minute)) {
		return c.accessToken, nil
	}

	assertion, err := c.signedAssertion()
	if err != nil {
		return "", err
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", &SendError{Message: "oauth2 token request: " + err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode != http.StatusOK {
		// The response can echo details about the key; keep only the status.
		return "", &SendError{
			Status:    resp.StatusCode,
			Message:   "oauth2 token request rejected",
			Retryable: resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests,
		}
	}
	var granted struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(payload, &granted); err != nil || granted.AccessToken == "" {
		return "", &SendError{Status: resp.StatusCode, Message: "oauth2 token response unreadable"}
	}
	lifetime := time.Duration(granted.ExpiresIn) * time.Second
	if lifetime <= 0 {
		lifetime = 55 * time.Minute
	}
	c.accessToken = granted.AccessToken
	c.expiresAt = c.now().Add(lifetime)
	return c.accessToken, nil
}

// signedAssertion builds the RS256 JWT bearer assertion Google exchanges for an
// access token.
func (c *FCMClient) signedAssertion() (string, error) {
	issued := c.now().Unix()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iss":   c.account.ClientEmail,
		"scope": fcmScope,
		"aud":   c.tokenURI,
		"iat":   issued,
		"exp":   issued + 3600,
	})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("push: sign assertion: %w", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func parseRSAKey(pemText string) (*rsa.PrivateKey, error) {
	// Keys pasted into a single-line environment value carry literal "\n".
	pemText = strings.ReplaceAll(pemText, `\n`, "\n")
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("push: service account private_key is not PEM")
	}
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if key, ok := parsed.(*rsa.PrivateKey); ok {
			return key, nil
		}
		return nil, errors.New("push: service account private_key is not an RSA key")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("push: service account private_key could not be parsed")
	}
	return key, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
