package app

import (
	"fmt"
	"time"

	mockSMS "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/notification/sms/mock"
	vodafoneSMS "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/notification/sms/vodafone"
	identitySMS "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/adapter/sms"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

// newSMSCodeTexter builds the sender phone sign-in codes go out through. ttl is
// how long a code works, which is also how long the provider keeps trying to
// deliver it.
func newSMSCodeTexter(sms *SMSConfig, ttl time.Duration) (*identitySMS.CodeTexter, error) {
	if sms == nil {
		return nil, fmt.Errorf("SMS_PROVIDER is required")
	}
	var sender identitySMS.TextSender
	switch sms.Provider {
	case SMSProviderMock:
		sender = mockSMS.New(logger.Log)
	case SMSProviderVodafone:
		vodafone, err := vodafoneSMS.New(vodafoneSMS.Config{
			BaseURL: sms.Vodafone.BaseURL, TokenPath: sms.Vodafone.TokenPath, BasicAuth: sms.Vodafone.BasicAuth,
			Username: sms.Vodafone.Username, Password: sms.Vodafone.Password, SenderID: sms.Vodafone.SenderID,
			Validity: ttl,
		})
		if err != nil {
			return nil, err
		}
		sender = vodafone
	default:
		return nil, fmt.Errorf("unsupported SMS_PROVIDER %q", sms.Provider)
	}
	return identitySMS.NewCodeTexter(sender, sms.CodeMessage)
}

// codeTTL is how long a one-time code works. A Config built in code rather than
// by config.Load leaves it zero, which means the default.
func codeTTL(cfg *config.Config) time.Duration {
	if cfg.OneTimeCodeTTL == 0 {
		return config.DefaultOneTimeCodeTTL
	}
	return cfg.OneTimeCodeTTL
}
