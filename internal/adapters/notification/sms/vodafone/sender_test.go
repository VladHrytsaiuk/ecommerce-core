package vodafone

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeOBM stands in for the OBM API: it issues tokens and records sends.
type fakeOBM struct {
	mu            sync.Mutex
	tokenRequests int
	sends         []sendRequest
	sendTokens    []string
	// rejectToken is a token the send endpoint answers 401 for.
	rejectToken string
	sendStatus  int
	tokenStatus int
}

func (f *fakeOBM) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(DefaultTokenPath, func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.tokenRequests++
		if r.Header.Get("Authorization") != DefaultBasicAuth || r.FormValue("grant_type") != "password" || r.FormValue("username") != "store" || r.FormValue("password") != "secret" {
			t.Errorf("token request carried the wrong credentials")
		}
		if f.tokenStatus != 0 {
			w.WriteHeader(f.tokenStatus)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "token-" + string(rune('0'+f.tokenRequests)), "expires_in": 3600})
	})
	mux.HandleFunc(sendPath, func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		f.sendTokens = append(f.sendTokens, token)
		if token == f.rejectToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body sendRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("send body: %v", err)
		}
		f.sends = append(f.sends, body)
		if f.sendStatus != 0 {
			w.WriteHeader(f.sendStatus)
			return
		}
		_, _ = w.Write([]byte(`[{"id":"message-1"}]`))
	})
	return mux
}

func newTestSender(t *testing.T, obm *fakeOBM) *Sender {
	t.Helper()
	server := httptest.NewServer(obm.handler(t))
	t.Cleanup(server.Close)
	sender, err := New(Config{BaseURL: server.URL, Username: "store", Password: "secret", SenderID: 7364601, Validity: 10 * time.Minute, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return sender
}

func TestATextIsSentInTheShapeOBMExpects(t *testing.T) {
	obm := &fakeOBM{}
	sender := newTestSender(t, obm)

	if err := sender.Send(context.Background(), "+380501234567", "Your code: 123456"); err != nil {
		t.Fatal(err)
	}
	if len(obm.sends) != 1 {
		t.Fatalf("sends = %d, want 1", len(obm.sends))
	}
	sent := obm.sends[0]
	if len(sent.Receiver) != 1 || sent.Receiver[0] != "380501234567" {
		t.Fatalf("receiver = %v, want the digits without +", sent.Receiver)
	}
	if len(sent.Cascades) != 1 {
		t.Fatalf("cascades = %+v", sent.Cascades)
	}
	cascade := sent.Cascades[0]
	if cascade.Transport != "SMS" || cascade.SenderID != 7364601 || cascade.ValidityPeriod != "10" || cascade.MessageObject.Type != "SMS" || cascade.MessageObject.SMSMessage.Content != "Your code: 123456" {
		t.Fatalf("cascade = %+v", cascade)
	}
}

func TestATokenIsReusedUntilItNearlyExpires(t *testing.T) {
	obm := &fakeOBM{}
	sender := newTestSender(t, obm)
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	sender.now = func() time.Time { return now }

	for range 3 {
		if err := sender.Send(context.Background(), "+380501234567", "text"); err != nil {
			t.Fatal(err)
		}
	}
	if obm.tokenRequests != 1 {
		t.Fatalf("token requests = %d, want one token shared by every send", obm.tokenRequests)
	}
	// An hour-long token is renewed a minute before it runs out.
	now = now.Add(59*time.Minute + time.Second)
	if err := sender.Send(context.Background(), "+380501234567", "text"); err != nil {
		t.Fatal(err)
	}
	if obm.tokenRequests != 2 {
		t.Fatalf("token requests = %d, want a new token near the expiry", obm.tokenRequests)
	}
}

func TestARevokedTokenIsRenewedOnce(t *testing.T) {
	obm := &fakeOBM{rejectToken: "token-1"}
	sender := newTestSender(t, obm)

	if err := sender.Send(context.Background(), "+380501234567", "text"); err != nil {
		t.Fatalf("Send() = %v, want it to renew the token and succeed", err)
	}
	if obm.tokenRequests != 2 || len(obm.sends) != 1 || obm.sendTokens[1] != "token-2" {
		t.Fatalf("token requests %d, sends %d, tokens %v; want one renewal and one delivered send", obm.tokenRequests, len(obm.sends), obm.sendTokens)
	}
}

func TestProviderErrorsDoNotCarryTheNumberOrTheText(t *testing.T) {
	obm := &fakeOBM{sendStatus: http.StatusBadRequest}
	sender := newTestSender(t, obm)

	err := sender.Send(context.Background(), "+380501234567", "Your code: 123456")
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("Send() = %v, want the status reported", err)
	}
	for _, secret := range []string{"380501234567", "123456"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error %q contains %q", err, secret)
		}
	}
	obm = &fakeOBM{tokenStatus: http.StatusUnauthorized}
	if err := newTestSender(t, obm).Send(context.Background(), "+380501234567", "text"); err == nil || !strings.Contains(err.Error(), "token request answered 401") {
		t.Fatalf("Send() with rejected credentials = %v", err)
	}
}

func TestTheSenderRefusesWhatCannotWork(t *testing.T) {
	valid := Config{BaseURL: "https://a2p.example.test", Username: "store", Password: "secret", SenderID: 1, Validity: 10 * time.Minute}
	for name, mutate := range map[string]func(*Config){
		"plain http to a remote host": func(c *Config) { c.BaseURL = "http://a2p.example.test" },
		"a relative URL":              func(c *Config) { c.BaseURL = "a2p.example.test" },
		"no username":                 func(c *Config) { c.Username = "" },
		"no password":                 func(c *Config) { c.Password = "" },
		"no sender id":                func(c *Config) { c.SenderID = 0 },
		"a validity under a minute":   func(c *Config) { c.Validity = 30 * time.Second },
	} {
		config := valid
		mutate(&config)
		if _, err := New(config); err == nil {
			t.Fatalf("%s: New() succeeded", name)
		}
	}
	if _, err := New(Config{Username: "store", Password: "secret", SenderID: 1, Validity: time.Minute}); err != nil {
		t.Fatalf("New() with the default base URL = %v", err)
	}
	sender, _ := New(valid)
	for _, phone := range []string{"380501234567", "+38050", "+38050123456x"} {
		if err := sender.Send(context.Background(), phone, "text"); err == nil || !strings.Contains(err.Error(), "invalid phone number") {
			t.Fatalf("Send(%q) = %v, want refused before any request", phone, err)
		}
	}
}
