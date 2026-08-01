// Package sms містить інтеграцію з Vodafone OBM REST API v3 для відправки SMS.
package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	platformsms "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/sms"
)

// VodafoneConfig містить налаштування для підключення до Vodafone OBM.
type VodafoneConfig struct {
	BaseURL         string // напр. https://a2p.vodafone.ua
	TokenPath       string // напр. /customers-registration/api/v1/customer/oauth/token
	BasicAuthHeader string // напр. "Basic d2ViYXBwOndlYmFwcA==" (client credentials OBM)
	Username        string // логін OBM-акаунта
	Password        string // пароль OBM-акаунта
	SenderID        int    // ID імені відправника (напр. 7364601 = AQUAWHEEL)
	ValidityMinutes string // час життя повідомлення у хвилинах (напр. "2")
	StatusCheck     bool   // чи перевіряти статус доставки після відправки (лог DELIVERED/REJECTED)
}

// sendPath — ендпоінт відправки повідомлення (OBM REST API v3).
const sendPath = "/communication-event/api/communicationManagement/v3/communicationMessage/send"

// statusPath — ендпоінт перевірки статусу повідомлення.
const statusPath = "/communication-event/api/communicationManagement/v3/communicationMessage/status"

// statusCheckDelay — затримка перед перевіркою статусу (даємо OBM час обробити).
const statusCheckDelay = 5 * time.Second

// VodafoneSender реалізує platformsms.Sender через Vodafone OBM REST API v3.
type VodafoneSender struct {
	cfg        VodafoneConfig
	httpClient *http.Client
	logger     logger.Logger

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// NewVodafoneSender створює реального відправника SMS через Vodafone OBM.
func NewVodafoneSender(cfg VodafoneConfig, l logger.Logger) platformsms.Sender {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://a2p.vodafone.ua"
	}
	if cfg.TokenPath == "" {
		cfg.TokenPath = "/uaa/oauth/token"
	}
	if cfg.BasicAuthHeader == "" {
		// Стандартні client credentials OBM (webapp:webapp) з документації.
		cfg.BasicAuthHeader = "Basic d2ViYXBwOndlYmFwcA=="
	}
	if cfg.ValidityMinutes == "" {
		cfg.ValidityMinutes = "2"
	}
	return &VodafoneSender{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		logger:     l,
	}
}

// ==========================================
// DTO для OBM API
// ==========================================

type obmTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"` // у секундах
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// Вкладена схема тіла (deployment з /uaa/oauth/token, документ "АРІ v3+callback+carusel").
type obmSMSMessage struct {
	Content string `json:"content"`
}

type obmMessageObject struct {
	Type       string        `json:"type"` // "SMS"
	SMSMessage obmSMSMessage `json:"smsMessage"`
}

type obmCascade struct {
	Transport      string           `json:"transport"` // "SMS"
	SenderID       int              `json:"senderId"`
	ValidityPeriod string           `json:"validityPeriod"`
	MessageObject  obmMessageObject `json:"messageObject"`
}

type obmSendRequest struct {
	Receiver []string     `json:"receiver"`
	Cascades []obmCascade `json:"cascades"`
}

// --- Статус доставки (?type=FULL) ---

type obmStatusError struct {
	ErrorCode          string `json:"errorCode"`
	ErrorDescriptionEN string `json:"errorDescription_en"`
	ErrorDescriptionUK string `json:"errorDescription_uk"`
}

type obmStatusCascade struct {
	Channel string          `json:"channel"`
	Status  string          `json:"status"`
	Error   *obmStatusError `json:"error"`
}

type obmStatusResponse struct {
	MessageID string             `json:"messageId"`
	Status    string             `json:"status"` // SENT / DELIVERED / EXPIRED / REJECTED
	Error     *obmStatusError    `json:"error"`
	Cascades  []obmStatusCascade `json:"cascades"`
}

// ==========================================
// Реалізація Sender
// ==========================================

// SendVerificationCode формує текст повідомлення та відправляє його через OBM.
func (s *VodafoneSender) SendVerificationCode(ctx context.Context, phone, code string) error {
	msisdn := normalizeMSISDN(phone)
	if msisdn == "" {
		return fmt.Errorf("invalid phone number for SMS: %q", phone)
	}

	content := fmt.Sprintf("Код підтвердження: %s\nДійсний протягом %s хв.", code, s.cfg.ValidityMinutes)

	return s.send(ctx, msisdn, content)
}

func (s *VodafoneSender) send(ctx context.Context, msisdn, content string) error {
	token, err := s.getToken(ctx)
	if err != nil {
		return fmt.Errorf("obm auth failed: %w", err)
	}

	reqBody := obmSendRequest{
		Receiver: []string{msisdn},
		Cascades: []obmCascade{
			{
				Transport:      "SMS",
				SenderID:       s.cfg.SenderID,
				ValidityPeriod: s.cfg.ValidityMinutes,
				MessageObject: obmMessageObject{
					Type:       "SMS",
					SMSMessage: obmSMSMessage{Content: content},
				},
			},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal obm send request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.BaseURL+sendPath, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to build obm send request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("obm send request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.logger.Errorw("obm send returned non-2xx",
			"status", resp.StatusCode,
			"body", string(body),
			"msisdn", msisdn,
		)
		return fmt.Errorf("obm send failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	// Успіх: відповідь — масив із полем "id" повідомлення.
	var ok []struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ok)
	var msgID string
	if len(ok) > 0 {
		msgID = ok[0].ID
	}

	s.logger.Infow("✅ SMS accepted by Vodafone OBM",
		"msisdn", msisdn,
		"message_id", msgID,
	)

	// Перевірка фактичної доставки (OBM міг прийняти, але відхилити доставку).
	if s.cfg.StatusCheck && msgID != "" {
		s.scheduleStatusCheck(msgID, msisdn)
	}

	return nil
}

// scheduleStatusCheck у фоні чекає кілька секунд і логує реальний статус доставки.
func (s *VodafoneSender) scheduleStatusCheck(messageID, msisdn string) {
	go func() {
		time.Sleep(statusCheckDelay)

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		st, err := s.getStatus(ctx, messageID)
		if err != nil {
			s.logger.Warnw("could not fetch OBM delivery status",
				"error", err, "message_id", messageID)
			return
		}

		// Витягуємо найінформативнішу помилку: спершу з каскаду, потім з верхнього рівня.
		var errCode, errDesc string
		if len(st.Cascades) > 0 && st.Cascades[0].Error != nil {
			errCode = st.Cascades[0].Error.ErrorCode
			errDesc = st.Cascades[0].Error.ErrorDescriptionUK
		} else if st.Error != nil {
			errCode = st.Error.ErrorCode
			errDesc = st.Error.ErrorDescriptionUK
		}

		switch st.Status {
		case "DELIVERED":
			s.logger.Infow("📬 SMS DELIVERED", "msisdn", msisdn, "message_id", messageID)
		case "SENT":
			s.logger.Infow("📨 SMS handed to operator (SENT)", "msisdn", msisdn, "message_id", messageID)
		default: // REJECTED / EXPIRED
			s.logger.Errorw("❌ SMS NOT delivered",
				"status", st.Status,
				"errorCode", errCode,
				"errorDescription", errDesc,
				"msisdn", msisdn,
				"message_id", messageID,
			)
		}
	}()
}

// getStatus запитує розширений статус повідомлення (?type=FULL).
func (s *VodafoneSender) getStatus(ctx context.Context, messageID string) (*obmStatusResponse, error) {
	token, err := s.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("obm auth failed: %w", err)
	}

	u := fmt.Sprintf("%s%s?id=%s&type=FULL", s.cfg.BaseURL, statusPath, url.QueryEscape(messageID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build status request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("status request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status request returned %d: %s", resp.StatusCode, string(body))
	}

	var st obmStatusResponse
	if err := json.Unmarshal(body, &st); err != nil {
		return nil, fmt.Errorf("failed to parse status response: %w", err)
	}
	return &st, nil
}

// getToken повертає валідний access_token, кешуючи його до закінчення терміну дії.
func (s *VodafoneSender) getToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 60с буфера, щоб не використати токен, що ось-ось протермінується.
	if s.accessToken != "" && time.Now().Before(s.tokenExpiry.Add(-60*time.Second)) {
		return s.accessToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("username", s.cfg.Username)
	form.Set("password", s.cfg.Password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.BaseURL+s.cfg.TokenPath, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", s.cfg.BasicAuthHeader)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var tokenResp obmTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response (status=%d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || tokenResp.AccessToken == "" {
		return "", fmt.Errorf("token request rejected: status=%d error=%s desc=%s", resp.StatusCode, tokenResp.Error, tokenResp.ErrorDesc)
	}

	s.accessToken = tokenResp.AccessToken
	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 43199 // дефолт OBM (~12 годин)
	}
	s.tokenExpiry = time.Now().Add(time.Duration(expiresIn) * time.Second)

	return s.accessToken, nil
}

// normalizeMSISDN приводить номер до формату MSISDN, який очікує OBM (напр. 380991234567).
// Видаляє пробіли, дужки, дефіси, "+"; конвертує локальні формати (0XX..., 80XX...) у 380XX....
func normalizeMSISDN(phone string) string {
	var digits strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	d := digits.String()

	switch {
	case len(d) == 12 && strings.HasPrefix(d, "380"): // 380991234567
		return d
	case len(d) == 11 && strings.HasPrefix(d, "80"): // 80991234567
		return "3" + d
	case len(d) == 10 && strings.HasPrefix(d, "0"): // 0991234567
		return "38" + d
	case len(d) == 9: // 991234567
		return "380" + d
	default:
		return "" // невідомий формат — вважаємо некоректним
	}
}
