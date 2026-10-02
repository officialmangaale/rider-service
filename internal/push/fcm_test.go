package push

import (
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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testKeyPEM(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return key, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// fakeGoogle stands in for both oauth2.googleapis.com and fcm.googleapis.com.
type fakeGoogle struct {
	t          *testing.T
	server     *httptest.Server
	publicKey  *rsa.PublicKey
	tokenCalls atomic.Int32
	sendCalls  atomic.Int32

	// sendStatuses answers successive sends; the last one repeats.
	sendStatuses []int
	sendBody     string
	lastSend     map[string]any
	lastAuth     string
	claims       map[string]any
}

func newFakeGoogle(t *testing.T, pub *rsa.PublicKey) *fakeGoogle {
	f := &fakeGoogle{t: t, publicKey: pub, sendStatuses: []int{200}}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", f.handleToken)
	mux.HandleFunc("/v1/projects/rider-proj/messages:send", f.handleSend)
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeGoogle) handleToken(w http.ResponseWriter, r *http.Request) {
	f.tokenCalls.Add(1)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
		http.Error(w, "bad grant", 400)
		return
	}
	parts := strings.Split(r.Form.Get("assertion"), ".")
	if len(parts) != 3 {
		http.Error(w, "bad jwt", 400)
		return
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		http.Error(w, "bad sig", 400)
		return
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(f.publicKey, crypto.SHA256, digest[:], signature); err != nil {
		http.Error(w, "signature does not verify", 401)
		return
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	_ = json.Unmarshal(raw, &f.claims)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"access_token":"ya29.test-token","expires_in":3600,"token_type":"Bearer"}`))
}

func (f *fakeGoogle) handleSend(w http.ResponseWriter, r *http.Request) {
	n := int(f.sendCalls.Add(1))
	f.lastAuth = r.Header.Get("Authorization")
	body, _ := io.ReadAll(r.Body)
	var envelope struct {
		Message map[string]any `json:"message"`
	}
	_ = json.Unmarshal(body, &envelope)
	f.lastSend = envelope.Message

	status := f.sendStatuses[len(f.sendStatuses)-1]
	if n <= len(f.sendStatuses) {
		status = f.sendStatuses[n-1]
	}
	w.WriteHeader(status)
	if status >= 300 {
		_, _ = w.Write([]byte(f.sendBody))
		return
	}
	_, _ = w.Write([]byte(`{"name":"projects/rider-proj/messages/1"}`))
}

func newTestClient(t *testing.T, f *fakeGoogle, key string) *FCMClient {
	t.Helper()
	client, err := NewFCMClient(Config{
		Account: ServiceAccount{
			ProjectID:   "rider-proj",
			ClientEmail: "push@rider-proj.iam.gserviceaccount.com",
			PrivateKey:  key,
			TokenURI:    f.server.URL + "/token",
		},
		Endpoint: f.server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestSendAuthenticatesAndDeliversADataOnlyHighPriorityMessage(t *testing.T) {
	priv, pemKey := testKeyPEM(t)
	google := newFakeGoogle(t, &priv.PublicKey)
	client := newTestClient(t, google, pemKey)

	err := client.Send(context.Background(), Message{
		Token:       "device-token-1",
		Data:        map[string]string{"type": TypeOfferCreated, "request_id": "12"},
		TTL:         27 * time.Second,
		CollapseKey: "offer-12",
	})
	if err != nil {
		t.Fatal(err)
	}

	// The JWT assertion Google would have accepted.
	if google.claims["iss"] != "push@rider-proj.iam.gserviceaccount.com" ||
		google.claims["scope"] != "https://www.googleapis.com/auth/firebase.messaging" ||
		google.claims["aud"] != google.server.URL+"/token" {
		t.Fatalf("claims %v", google.claims)
	}
	exp, iat := google.claims["exp"].(float64), google.claims["iat"].(float64)
	if exp-iat != 3600 {
		t.Fatalf("assertion lifetime %v", exp-iat)
	}
	if google.lastAuth != "Bearer ya29.test-token" {
		t.Fatalf("auth header %q", google.lastAuth)
	}

	msg := google.lastSend
	if msg["token"] != "device-token-1" {
		t.Fatalf("token %v", msg["token"])
	}
	if _, hasNotification := msg["notification"]; hasNotification {
		// A notification block would let Android draw it itself, without the
		// Accept and Decline actions, and skip the app's background handler.
		t.Fatalf("Android delivery must be data-only: %v", msg)
	}
	data := msg["data"].(map[string]any)
	if data["type"] != TypeOfferCreated || data["request_id"] != "12" {
		t.Fatalf("data %v", data)
	}
	android := msg["android"].(map[string]any)
	if android["priority"] != "HIGH" || android["ttl"] != "27s" || android["collapse_key"] != "offer-12" {
		t.Fatalf("android %v", android)
	}
	// Without an alert the iOS delivery is a silent background message.
	apns := msg["apns"].(map[string]any)
	headers := apns["headers"].(map[string]any)
	if headers["apns-push-type"] != "background" || headers["apns-priority"] != "5" {
		t.Fatalf("apns headers %v", headers)
	}
}

func TestSendAlertsOnIOSWithTheActionCategory(t *testing.T) {
	priv, pemKey := testKeyPEM(t)
	google := newFakeGoogle(t, &priv.PublicKey)
	client := newTestClient(t, google, pemKey)

	err := client.Send(context.Background(), Message{
		Token:        "ios-token",
		Data:         map[string]string{"type": TypeOfferCreated},
		TTL:          30 * time.Second,
		Alert:        &Alert{Title: "New delivery request", Body: "#1 · Kitchen"},
		APNSCategory: APNSCategoryOffer,
	})
	if err != nil {
		t.Fatal(err)
	}
	apns := google.lastSend["apns"].(map[string]any)
	headers := apns["headers"].(map[string]any)
	aps := apns["payload"].(map[string]any)["aps"].(map[string]any)
	if headers["apns-push-type"] != "alert" || headers["apns-priority"] != "10" {
		t.Fatalf("apns headers %v", headers)
	}
	if aps["category"] != APNSCategoryOffer || aps["interruption-level"] != "time-sensitive" {
		t.Fatalf("aps %v", aps)
	}
	if aps["alert"].(map[string]any)["body"] != "#1 · Kitchen" {
		t.Fatalf("aps alert %v", aps["alert"])
	}
}

func TestTheAccessTokenIsCachedAcrossSends(t *testing.T) {
	priv, pemKey := testKeyPEM(t)
	google := newFakeGoogle(t, &priv.PublicKey)
	client := newTestClient(t, google, pemKey)

	for i := 0; i < 3; i++ {
		if err := client.Send(context.Background(), Message{Token: "t", Data: map[string]string{"a": "b"}}); err != nil {
			t.Fatal(err)
		}
	}
	if google.tokenCalls.Load() != 1 || google.sendCalls.Load() != 3 {
		t.Fatalf("token calls=%d send calls=%d", google.tokenCalls.Load(), google.sendCalls.Load())
	}
}

func TestAnExpiredAccessTokenIsRenewed(t *testing.T) {
	priv, pemKey := testKeyPEM(t)
	google := newFakeGoogle(t, &priv.PublicKey)
	client := newTestClient(t, google, pemKey)
	now := time.Now()
	client.now = func() time.Time { return now }

	if err := client.Send(context.Background(), Message{Token: "t", Data: map[string]string{"a": "b"}}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if err := client.Send(context.Background(), Message{Token: "t", Data: map[string]string{"a": "b"}}); err != nil {
		t.Fatal(err)
	}
	if google.tokenCalls.Load() != 2 {
		t.Fatalf("token calls=%d", google.tokenCalls.Load())
	}
}

func TestRetiredTokensAreReportedSoTheyCanBeForgotten(t *testing.T) {
	priv, pemKey := testKeyPEM(t)
	cases := map[string]struct {
		status int
		body   string
		want   bool
	}{
		"unregistered detail": {404, `{"error":{"code":404,"status":"NOT_FOUND","message":"Requested entity was not found.","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`, true},
		"plain not found":     {404, `{"error":{"status":"NOT_FOUND"}}`, true},
		"malformed token":     {400, `{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"The registration token is not a valid FCM registration token"}}`, true},
		// A payload mistake must never unsubscribe riders.
		"bad payload":    {400, `{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"Invalid value at 'message.data'"}}`, false},
		"sender denied":  {403, `{"error":{"code":403,"status":"PERMISSION_DENIED","message":"caller lacks permission"}}`, false},
		"quota":          {429, `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"quota"}}`, false},
		"server trouble": {503, `{"error":{"code":503,"status":"UNAVAILABLE","message":"try later"}}`, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			google := newFakeGoogle(t, &priv.PublicKey)
			google.sendStatuses = []int{tc.status}
			google.sendBody = tc.body
			client := newTestClient(t, google, pemKey)
			err := client.Send(context.Background(), Message{Token: "t", Data: map[string]string{"a": "b"}})
			if got := errors.Is(err, ErrUnregistered); got != tc.want {
				t.Fatalf("ErrUnregistered=%v want %v (err=%v)", got, tc.want, err)
			}
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestTransientFailuresAreRetriedOnceAndClientErrorsAreNot(t *testing.T) {
	priv, pemKey := testKeyPEM(t)

	google := newFakeGoogle(t, &priv.PublicKey)
	google.sendStatuses = []int{503, 200}
	google.sendBody = `{"error":{"status":"UNAVAILABLE"}}`
	client := newTestClient(t, google, pemKey)
	if err := client.Send(context.Background(), Message{Token: "t", Data: map[string]string{"a": "b"}}); err != nil {
		t.Fatalf("a retry should have succeeded: %v", err)
	}
	if google.sendCalls.Load() != 2 {
		t.Fatalf("sends=%d", google.sendCalls.Load())
	}

	google = newFakeGoogle(t, &priv.PublicKey)
	google.sendStatuses = []int{400}
	google.sendBody = `{"error":{"status":"INVALID_ARGUMENT","message":"bad"}}`
	client = newTestClient(t, google, pemKey)
	err := client.Send(context.Background(), Message{Token: "t", Data: map[string]string{"a": "b"}})
	var sendErr *SendError
	if !errors.As(err, &sendErr) || sendErr.Retryable || google.sendCalls.Load() != 1 {
		t.Fatalf("err=%v sends=%d", err, google.sendCalls.Load())
	}
}

func TestARevokedAccessTokenIsReplacedOnce(t *testing.T) {
	priv, pemKey := testKeyPEM(t)
	google := newFakeGoogle(t, &priv.PublicKey)
	google.sendStatuses = []int{401, 200}
	google.sendBody = `{"error":{"status":"UNAUTHENTICATED"}}`
	client := newTestClient(t, google, pemKey)

	if err := client.Send(context.Background(), Message{Token: "t", Data: map[string]string{"a": "b"}}); err != nil {
		t.Fatal(err)
	}
	if google.tokenCalls.Load() != 2 {
		t.Fatalf("a 401 should mint a fresh token; token calls=%d", google.tokenCalls.Load())
	}
}

func TestSendRejectsAnEmptyDeviceToken(t *testing.T) {
	priv, pemKey := testKeyPEM(t)
	google := newFakeGoogle(t, &priv.PublicKey)
	client := newTestClient(t, google, pemKey)
	if err := client.Send(context.Background(), Message{Data: map[string]string{"a": "b"}}); err == nil {
		t.Fatal("expected an error")
	}
	if google.sendCalls.Load() != 0 || google.tokenCalls.Load() != 0 {
		t.Fatal("nothing should be sent for an empty token")
	}
}

func TestServiceAccountParsing(t *testing.T) {
	_, pemKey := testKeyPEM(t)
	raw, _ := json.Marshal(map[string]string{
		"project_id": "rider-proj", "client_email": "a@b.iam.gserviceaccount.com", "private_key": pemKey,
	})

	fromJSON, err := ParseServiceAccount(string(raw))
	if err != nil || fromJSON.ProjectID != "rider-proj" {
		t.Fatalf("json: %+v %v", fromJSON, err)
	}
	fromBase64, err := ParseServiceAccount(base64.StdEncoding.EncodeToString(raw))
	if err != nil || fromBase64.ClientEmail != "a@b.iam.gserviceaccount.com" {
		t.Fatalf("base64: %+v %v", fromBase64, err)
	}
	for _, bad := range []string{"", "   ", "not json or base64!!", `{"project_id":"x"}`, `{`} {
		if _, err := ParseServiceAccount(bad); err == nil {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}

func TestClientConstructionValidatesTheKeyAndProject(t *testing.T) {
	_, pemKey := testKeyPEM(t)
	if _, err := NewFCMClient(Config{Account: ServiceAccount{ClientEmail: "a@b", PrivateKey: pemKey}}); err == nil {
		t.Fatal("a project id is required")
	}
	if _, err := NewFCMClient(Config{Account: ServiceAccount{ProjectID: "p", ClientEmail: "a@b", PrivateKey: "garbage"}}); err == nil {
		t.Fatal("an unparseable key must fail at start-up")
	}
	// A key pasted into a one-line environment variable carries literal \n.
	oneLine := strings.ReplaceAll(pemKey, "\n", `\n`)
	client, err := NewFCMClient(Config{ProjectID: "override", Account: ServiceAccount{ProjectID: "p", ClientEmail: "a@b", PrivateKey: oneLine}})
	if err != nil {
		t.Fatal(err)
	}
	if client.ProjectID() != "override" {
		t.Fatalf("FCM_PROJECT_ID should win, got %q", client.ProjectID())
	}
}
