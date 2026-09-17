// Package vodafone sends SMS through Vodafone Ukraine's OBM (A2P) REST API v3.
package vodafone

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURL   = "https://a2p.vodafone.ua"
	DefaultTokenPath = "/uaa/oauth/token"
	// DefaultBasicAuth is the client credential OBM's documentation gives every
	// customer (webapp:webapp). The account's own username and password are
	// what authenticate it.
	DefaultBasicAuth = "Basic d2ViYXBwOndlYmFwcA=="

	sendPath = "/communication-event/api/communicationManagement/v3/communicationMessage/send"

	// requestTimeout bounds each call. A text is sent while the customer waits
	// for the response to their request for a code.
	requestTimeout = 10 * time.Second
	// maxResponseBytes bounds what is read from a response; OBM's are a few
	// hundred bytes.
	maxResponseBytes = 64 << 10
	// tokenRefreshMargin renews a token this long before it expires, so one is
	// never used in the moment it runs out.
	tokenRefreshMargin = time.Minute
	// defaultTokenLifetime applies when OBM omits expires_in; its tokens last
	// about twelve hours.
	defaultTokenLifetime = 12 * time.Hour
)

type Config struct {
	BaseURL   string
	TokenPath string
	BasicAuth string
	Username  string
	Password  string
	// SenderID is the OBM id of the approved sender name the text comes from.
	SenderID int
	// Validity is how long the operator keeps trying to deliver a text. It
	// should match how long the code in it works: a code delivered later is
	// useless.
	Validity time.Duration
	// HTTPClient replaces the default client, for tests.
	HTTPClient *http.Client
}

// Sender sends texts. It is safe for concurrent use; the access token is
// shared and renewed by one caller at a time.
type Sender struct {
	baseURL   string
	tokenURL  string
	basicAuth string
	username  string
	password  string
	senderID  int
	validity  string
	client    *http.Client
	now       func() time.Time

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

func New(cfg Config) (*Sender, error) {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if strings.TrimSpace(cfg.TokenPath) == "" {
		cfg.TokenPath = DefaultTokenPath
	}
	if strings.TrimSpace(cfg.BasicAuth) == "" {
		cfg.BasicAuth = DefaultBasicAuth
	}
	if err := checkBaseURL(cfg.BaseURL); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Username) == "" || cfg.Password == "" || cfg.SenderID <= 0 {
		return nil, fmt.Errorf("vodafone obm username, password and sender id are required")
	}
	if cfg.Validity < time.Minute {
		return nil, fmt.Errorf("vodafone obm message validity must be at least a minute")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	return &Sender{
		baseURL:   cfg.BaseURL,
		tokenURL:  cfg.BaseURL + "/" + strings.TrimLeft(cfg.TokenPath, "/"),
		basicAuth: cfg.BasicAuth,
		username:  strings.TrimSpace(cfg.Username),
		password:  cfg.Password,
		senderID:  cfg.SenderID,
		validity:  strconv.Itoa(int(cfg.Validity / time.Minute)),
		client:    client,
		now:       time.Now,
	}, nil
}

// checkBaseURL requires HTTPS, because the request carries the account's
// password and the text carries a sign-in code. Plain HTTP is allowed only to a
// loopback address, which is where a test server listens.
func checkBaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("vodafone obm base URL %q is not an absolute URL", raw)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if ip := net.ParseIP(parsed.Hostname()); parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || ip != nil && ip.IsLoopback()) {
		return nil
	}
	return fmt.Errorf("vodafone obm base URL must use https")
}

// errUnauthorized is a send refused for its token, which is then renewed once.
var errUnauthorized = errors.New("vodafone obm refused the access token")

// Send delivers text to phone, a number in E.164 form. Errors name the HTTP
// status and never the number or the text, which carries a sign-in code.
func (s *Sender) Send(ctx context.Context, phone, text string) error {
	msisdn, ok := msisdn(phone)
	if !ok {
		return fmt.Errorf("vodafone obm: invalid phone number")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("vodafone obm: empty message")
	}
	err := s.send(ctx, msisdn, text)
	if errors.Is(err, errUnauthorized) {
		// OBM can revoke a token before the expiry it announced.
		s.forgetToken()
		err = s.send(ctx, msisdn, text)
	}
	return err
}

type sendRequest struct {
	Receiver []string  `json:"receiver"`
	Cascades []cascade `json:"cascades"`
}

type cascade struct {
	Transport      string        `json:"transport"`
	SenderID       int           `json:"senderId"`
	ValidityPeriod string        `json:"validityPeriod"`
	MessageObject  messageObject `json:"messageObject"`
}

type messageObject struct {
	Type       string     `json:"type"`
	SMSMessage smsMessage `json:"smsMessage"`
}

type smsMessage struct {
	Content string `json:"content"`
}

func (s *Sender) send(ctx context.Context, msisdn, text string) error {
	token, err := s.accessToken(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(sendRequest{
		Receiver: []string{msisdn},
		Cascades: []cascade{{
			Transport: "SMS", SenderID: s.senderID, ValidityPeriod: s.validity,
			MessageObject: messageObject{Type: "SMS", SMSMessage: smsMessage{Content: text}},
		}},
	})
	if err != nil {
		return fmt.Errorf("vodafone obm: encode message: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+sendPath, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("vodafone obm: build send request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("vodafone obm: send: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		return errUnauthorized
	case response.StatusCode < 200 || response.StatusCode > 299:
		return fmt.Errorf("vodafone obm: send answered %d", response.StatusCode)
	}
	return nil
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// accessToken returns a token that is still valid, fetching one if needed. The
// lock is held across the fetch so concurrent sends wait for one token rather
// than each fetching their own.
func (s *Sender) accessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && s.now().Before(s.tokenExpiry.Add(-tokenRefreshMargin)) {
		return s.token, nil
	}
	form := url.Values{"grant_type": {"password"}, "username": {s.username}, "password": {s.password}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("vodafone obm: build token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", s.basicAuth)
	response, err := s.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("vodafone obm: token: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	var body tokenResponse
	decodeErr := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&body)
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return "", fmt.Errorf("vodafone obm: token request answered %d", response.StatusCode)
	}
	if decodeErr != nil || body.AccessToken == "" {
		return "", fmt.Errorf("vodafone obm: token response has no access token")
	}
	lifetime := time.Duration(body.ExpiresIn) * time.Second
	if lifetime <= 0 {
		lifetime = defaultTokenLifetime
	}
	s.token, s.tokenExpiry = body.AccessToken, s.now().Add(lifetime)
	return s.token, nil
}

func (s *Sender) forgetToken() {
	s.mu.Lock()
	s.token = ""
	s.mu.Unlock()
}

// msisdn turns an E.164 number into the digits-only form OBM expects.
func msisdn(phone string) (string, bool) {
	digits, ok := strings.CutPrefix(strings.TrimSpace(phone), "+")
	if !ok || len(digits) < 8 || len(digits) > 15 || strings.Trim(digits, "0123456789") != "" {
		return "", false
	}
	return digits, true
}
