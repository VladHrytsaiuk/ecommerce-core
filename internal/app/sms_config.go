package app

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
)

// SMS providers SMS_PROVIDER accepts.
const (
	SMSProviderMock     = "mock"
	SMSProviderVodafone = "vodafone"
)

// SMSConfig is how phone sign-in codes are sent. It exists only where
// SMS_PROVIDER is set, and only a store offering phone_code may set it.
type SMSConfig struct {
	Provider string
	// AllowedCountryCodes are the calling codes, without "+", texts may go to.
	AllowedCountryCodes []string
	// HourlyLimit caps the texts the whole store sends in an hour.
	HourlyLimit int
	// CodeMessage is the text, with {code} and optionally {minutes}.
	CodeMessage string
	Vodafone    VodafoneOBMConfig
}

type VodafoneOBMConfig struct {
	BaseURL   string
	TokenPath string
	BasicAuth string
	Username  string
	Password  string
	SenderID  int
}

func newSMSConfig(cfg *config.Config) *SMSConfig {
	provider := strings.ToLower(strings.TrimSpace(cfg.SMSProvider))
	if provider == "" {
		return nil
	}
	codes := make([]string, 0, len(cfg.SMSAllowedCountryCodes))
	for _, code := range cfg.SMSAllowedCountryCodes {
		if code = strings.TrimPrefix(strings.TrimSpace(code), "+"); code != "" {
			codes = append(codes, code)
		}
	}
	return &SMSConfig{
		Provider:            provider,
		AllowedCountryCodes: codes,
		HourlyLimit:         cfg.SMSHourlyLimit,
		CodeMessage:         strings.TrimSpace(cfg.SMSCodeMessage),
		Vodafone: VodafoneOBMConfig{
			BaseURL: strings.TrimSpace(cfg.VodafoneOBMBaseURL), TokenPath: strings.TrimSpace(cfg.VodafoneOBMTokenPath),
			BasicAuth: strings.TrimSpace(cfg.VodafoneOBMBasicAuth), Username: strings.TrimSpace(cfg.VodafoneOBMUsername),
			Password: cfg.VodafoneOBMPassword, SenderID: cfg.VodafoneOBMSenderID,
		},
	}
}

// maxSMSHourlyLimit is a sanity bound: a store sending more sign-in texts an
// hour than this has either a typo or an incident.
const maxSMSHourlyLimit = 100000

// validate reports everything wrong at once, like the module checks.
func (c *SMSConfig) validate() error {
	problems := make([]string, 0)
	switch c.Provider {
	case SMSProviderMock:
	case SMSProviderVodafone:
		if c.Vodafone.Username == "" || c.Vodafone.Password == "" || c.Vodafone.SenderID <= 0 {
			problems = append(problems, "SMS_PROVIDER=vodafone requires VODAFONE_OBM_USERNAME, VODAFONE_OBM_PASSWORD and VODAFONE_OBM_SENDER_ID")
		}
	default:
		problems = append(problems, fmt.Sprintf("SMS_PROVIDER %q is not supported; valid providers are %s, %s", c.Provider, SMSProviderMock, SMSProviderVodafone))
	}
	if len(c.AllowedCountryCodes) == 0 {
		problems = append(problems, "SMS_ALLOWED_COUNTRY_CODES is required: the calling codes texts may be sent to, such as 380")
	}
	for _, code := range c.AllowedCountryCodes {
		if len(code) > 3 || code[0] == '0' || strings.Trim(code, "0123456789") != "" {
			problems = append(problems, fmt.Sprintf("SMS_ALLOWED_COUNTRY_CODES contains %q, which is not a calling code", code))
		}
	}
	if c.HourlyLimit < 1 || c.HourlyLimit > maxSMSHourlyLimit {
		problems = append(problems, fmt.Sprintf("SMS_HOURLY_LIMIT must be between 1 and %d", maxSMSHourlyLimit))
	}
	if strings.Count(c.CodeMessage, "{code}") != 1 || utf8.RuneCountInString(c.CodeMessage) > 300 {
		problems = append(problems, "SMS_CODE_MESSAGE must contain {code} exactly once and be at most 300 characters")
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}
