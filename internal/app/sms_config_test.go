package app

import (
	"strings"
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
)

func phoneCodeConfig() *SMSConfig {
	return &SMSConfig{Provider: SMSProviderMock, AllowedCountryCodes: []string{"380"}, HourlyLimit: 100, CodeMessage: "Code: {code}"}
}

func TestPhoneCodesAndTheirSMSSettingsComeTogether(t *testing.T) {
	storeConfig, err := NewStoreConfig(validConfig())
	if err != nil {
		t.Fatal(err)
	}
	storeConfig.AuthMethods = AuthMethodSet{AuthMethodPhoneCode: {}}
	if err := storeConfig.Validate(); err == nil || !strings.Contains(err.Error(), "requires SMS_PROVIDER") {
		t.Fatalf("Validate(phone_code without SMS) = %v", err)
	}
	storeConfig.SMS = phoneCodeConfig()
	if err := storeConfig.Validate(); err != nil {
		t.Fatalf("Validate(phone_code with SMS) = %v", err)
	}
	storeConfig.AuthMethods = AuthMethodSet{AuthMethodPassword: {}}
	if err := storeConfig.Validate(); err == nil || !strings.Contains(err.Error(), "does not include phone_code") {
		t.Fatalf("Validate(SMS without phone_code) = %v", err)
	}
}

func TestSMSSettingsThatCannotWorkAreAllReported(t *testing.T) {
	for name, testCase := range map[string]struct {
		mutate func(*SMSConfig)
		want   string
	}{
		"an unknown provider":      {func(c *SMSConfig) { c.Provider = "twilio" }, "is not supported"},
		"vodafone without account": {func(c *SMSConfig) { c.Provider = SMSProviderVodafone }, "VODAFONE_OBM_USERNAME"},
		"no countries":             {func(c *SMSConfig) { c.AllowedCountryCodes = nil }, "SMS_ALLOWED_COUNTRY_CODES is required"},
		"a malformed country":      {func(c *SMSConfig) { c.AllowedCountryCodes = []string{"UA"} }, "not a calling code"},
		"no hourly cap":            {func(c *SMSConfig) { c.HourlyLimit = 0 }, "SMS_HOURLY_LIMIT"},
		"a message with no code":   {func(c *SMSConfig) { c.CodeMessage = "Welcome" }, "SMS_CODE_MESSAGE"},
	} {
		t.Run(name, func(t *testing.T) {
			config := phoneCodeConfig()
			testCase.mutate(config)
			if err := config.validate(); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("validate() = %v, want an error mentioning %q", err, testCase.want)
			}
		})
	}
	vodafone := phoneCodeConfig()
	vodafone.Provider = SMSProviderVodafone
	vodafone.Vodafone = VodafoneOBMConfig{Username: "store", Password: "secret", SenderID: 7364601}
	if err := vodafone.validate(); err != nil {
		t.Fatalf("validate(vodafone with its account) = %v", err)
	}
}

func TestTheSMSConfigIsReadFromTheEnvironmentSettings(t *testing.T) {
	cfg := validConfig()
	cfg.AuthMethods = []string{"password", "phone_code"}
	cfg.SMSProvider, cfg.SMSAllowedCountryCodes, cfg.SMSHourlyLimit, cfg.SMSCodeMessage = " Mock ", []string{"+380", " 48 "}, 20, "Code {code}"
	storeConfig, err := NewStoreConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	sms := storeConfig.SMS
	if sms == nil || sms.Provider != SMSProviderMock || strings.Join(sms.AllowedCountryCodes, ",") != "380,48" || sms.HourlyLimit != 20 {
		t.Fatalf("SMS = %+v", sms)
	}
	if _, err := newSMSCodeTexter(sms, codeTTL(cfg)); err != nil {
		t.Fatalf("newSMSCodeTexter() = %v", err)
	}
}

func TestCheckoutCannotRequireAVerificationTheStoreCannotDo(t *testing.T) {
	base := func(t *testing.T, methods []string, modules []string) *config.Config {
		t.Helper()
		cfg := validConfig()
		cfg.AuthMethods = methods
		cfg.EnabledModules = modules
		return cfg
	}
	withNotifications := withRequiredModules(string(ModuleNotifications))

	cfg := base(t, []string{"password"}, withRequiredModules())
	cfg.CheckoutRequireVerifiedEmail = true
	if _, err := NewStoreConfig(cfg); err == nil || !strings.Contains(err.Error(), "CHECKOUT_REQUIRE_VERIFIED_EMAIL") {
		t.Fatalf("NewStoreConfig(password without notifications) = %v, want refused", err)
	}
	cfg = base(t, []string{"password"}, withNotifications)
	cfg.CheckoutRequireVerifiedEmail = true
	if _, err := NewStoreConfig(cfg); err != nil {
		t.Fatalf("NewStoreConfig(password with notifications) = %v", err)
	}
	cfg = base(t, []string{"email_code"}, withNotifications)
	cfg.CheckoutRequireVerifiedEmail = true
	if _, err := NewStoreConfig(cfg); err != nil {
		t.Fatalf("NewStoreConfig(email_code) = %v", err)
	}

	cfg = base(t, []string{"password"}, withNotifications)
	cfg.CheckoutRequireVerifiedPhone = true
	if _, err := NewStoreConfig(cfg); err == nil || !strings.Contains(err.Error(), "CHECKOUT_REQUIRE_VERIFIED_PHONE") {
		t.Fatalf("NewStoreConfig(verified phone without phone_code) = %v, want refused", err)
	}
	cfg = base(t, []string{"password", "phone_code"}, withNotifications)
	cfg.CheckoutRequireVerifiedPhone = true
	cfg.SMSProvider, cfg.SMSAllowedCountryCodes, cfg.SMSHourlyLimit, cfg.SMSCodeMessage = "mock", []string{"380"}, 50, "Code {code}"
	if _, err := NewStoreConfig(cfg); err != nil {
		t.Fatalf("NewStoreConfig(phone_code) = %v", err)
	}
}
