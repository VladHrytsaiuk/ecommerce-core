package sms

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"go.uber.org/zap"
)

func TestNormalizeMSISDN(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"plus_380", "+380501234567", "380501234567"},
		{"plain_380", "380501234567", "380501234567"},
		{"local_0", "0501234567", "380501234567"},
		{"with_spaces", "+38 050 123 45 67", "380501234567"},
		{"with_dashes_parens", "+38(050)123-45-67", "380501234567"},
		{"eighty_prefix", "80501234567", "380501234567"},
		{"nine_digits", "501234567", "380501234567"},
		{"too_short", "12345", ""},
		{"empty", "", ""},
		{"garbage", "abc", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeMSISDN(tc.input); got != tc.want {
				t.Errorf("normalizeMSISDN(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
	t.Run("manual_test", func(t *testing.T) {
		assert.Equal(t, "", normalizeMSISDN("A-B-C"))
	})
}

type noopLogger struct{}

func (n *noopLogger) Debug(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Info(msg string, fields ...zap.Field)        {}
func (n *noopLogger) Warn(msg string, fields ...zap.Field)        {}
func (n *noopLogger) Error(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Fatal(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Debugf(template string, args ...interface{}) {}
func (n *noopLogger) Infof(template string, args ...interface{})  {}
func (n *noopLogger) Warnf(template string, args ...interface{})  {}
func (n *noopLogger) Errorf(template string, args ...interface{}) {}
func (n *noopLogger) Fatalf(template string, args ...interface{}) {}
func (n *noopLogger) Debugw(msg string, kvs ...interface{})       {}
func (n *noopLogger) Infow(msg string, kvs ...interface{})        {}
func (n *noopLogger) Warnw(msg string, kvs ...interface{})        {}
func (n *noopLogger) Errorw(msg string, kvs ...interface{})       {}
func (n *noopLogger) Fatalw(msg string, kvs ...interface{})       {}
func (n *noopLogger) With(fields ...zap.Field) logger.Logger      { return n }
func (n *noopLogger) Sync() error                                 { return nil }

func TestVodafoneSender_SendVerificationCode(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		requestCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			if requestCount == 1 {
				// Token request
				assert.Equal(t, "/token", r.URL.Path)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"access_token": "test-token", "expires_in": 3600}`))
			} else if requestCount == 2 {
				// Send request
				assert.Equal(t, "/communication-event/api/communicationManagement/v3/communicationMessage/send", r.URL.Path)
				assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`[{"id": "msg-123"}]`))
			}
		}))
		defer server.Close()

		cfg := VodafoneConfig{
			BaseURL:     server.URL,
			TokenPath:   "/token",
			Username:    "user",
			Password:    "pass",
			StatusCheck: false,
		}
		sender := NewVodafoneSender(cfg, &noopLogger{})

		err := sender.SendVerificationCode(context.Background(), "+380501234567", "123456")
		assert.NoError(t, err)
		assert.Equal(t, 2, requestCount)
	})

	t.Run("InvalidPhone", func(t *testing.T) {
		cfg := VodafoneConfig{}
		sender := NewVodafoneSender(cfg, &noopLogger{})
		err := sender.SendVerificationCode(context.Background(), "invalid", "123456")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid phone number")
	})

	t.Run("TokenFail", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "invalid_client"}`))
		}))
		defer server.Close()

		cfg := VodafoneConfig{BaseURL: server.URL, TokenPath: "/token"}
		sender := NewVodafoneSender(cfg, &noopLogger{})

		err := sender.SendVerificationCode(context.Background(), "+380501234567", "123456")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "obm auth failed")
	})

	t.Run("SendFail", func(t *testing.T) {
		requestCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			if requestCount == 1 {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"access_token": "test-token", "expires_in": 3600}`))
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`Internal Server Error`))
			}
		}))
		defer server.Close()

		cfg := VodafoneConfig{BaseURL: server.URL, TokenPath: "/token"}
		sender := NewVodafoneSender(cfg, &noopLogger{})

		err := sender.SendVerificationCode(context.Background(), "+380501234567", "123456")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "obm send failed")
	})
}
