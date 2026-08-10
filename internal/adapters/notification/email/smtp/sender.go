// Package smtp provides the standard-library SMTP implementation of the
// provider-neutral Notifications email port.
package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	notificationsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
)

// operationTimeout is deliberately far below the five-minute notification job
// lease. A stalled SMTP server therefore cannot outlive its delivery owner and
// create a concurrent resend when the outbox lease is reclaimed.
const operationTimeout = notificationsDomain.MaxEmailOperationTimeout

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	// TLSMode is "starttls" (required, default) or "implicit" for SMTPS/465.
	TLSMode string
	// TLSServerName overrides Host only when the SMTP endpoint is reached by an
	// IP address or internal alias while its certificate uses another name.
	TLSServerName string
}

type Sender struct {
	host      string
	port      int
	from      string
	auth      smtp.Auth
	tlsMode   string
	tlsConfig *tls.Config
}

func New(cfg Config) (*Sender, error) {
	cfg.Host, cfg.From = strings.TrimSpace(cfg.Host), strings.TrimSpace(cfg.From)
	cfg.TLSMode, cfg.TLSServerName = strings.ToLower(strings.TrimSpace(cfg.TLSMode)), strings.TrimSpace(cfg.TLSServerName)
	if cfg.Host == "" || cfg.Port < 1 || cfg.Port > 65535 || cfg.From == "" {
		return nil, fmt.Errorf("SMTP host, port and from address are required")
	}
	from, err := mail.ParseAddress(cfg.From)
	if err != nil || from.Address == "" {
		return nil, fmt.Errorf("invalid SMTP from address")
	}
	if strings.TrimSpace(cfg.Username) != "" && strings.TrimSpace(cfg.Password) == "" {
		return nil, fmt.Errorf("SMTP password is required when SMTP username is set")
	}
	if cfg.TLSMode == "" {
		cfg.TLSMode = "starttls"
	}
	if cfg.TLSMode != "starttls" && cfg.TLSMode != "implicit" {
		return nil, fmt.Errorf("SMTP TLS mode must be starttls or implicit")
	}
	if cfg.TLSServerName == "" {
		cfg.TLSServerName = cfg.Host
	}
	sender := &Sender{host: cfg.Host, port: cfg.Port, from: from.Address, tlsMode: cfg.TLSMode, tlsConfig: &tls.Config{ServerName: cfg.TLSServerName, MinVersion: tls.VersionTLS12}}
	if strings.TrimSpace(cfg.Username) != "" {
		sender.auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	return sender, nil
}

func (*Sender) Code() string { return "smtp" }

// Send runs outside the database transaction. net/smtp does not expose a
// context-aware API, so the context is checked before starting network I/O.
func (s *Sender) Send(ctx context.Context, message notificationsDomain.EmailMessage) (notificationsDomain.DeliveryReceipt, error) {
	if err := ctx.Err(); err != nil {
		return notificationsDomain.DeliveryReceipt{}, err
	}
	to, err := mail.ParseAddress(strings.TrimSpace(message.To))
	if err != nil || to.Address == "" {
		return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("invalid recipient address")
	}
	body, contentType := message.HTML, "text/html; charset=UTF-8"
	if body == "" {
		body, contentType = message.Text, "text/plain; charset=UTF-8"
	}
	if body == "" || strings.TrimSpace(message.Subject) == "" {
		return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("email subject and body are required")
	}
	if strings.ContainsAny(message.Subject, "\r\n") || strings.ContainsAny(message.MessageID, "\r\n") {
		return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("email headers contain an invalid newline")
	}
	headers := []string{
		"From: " + s.from,
		"To: " + to.Address,
		"Subject: " + message.Subject,
		"MIME-Version: 1.0",
		"Content-Type: " + contentType,
	}
	if strings.TrimSpace(message.MessageID) != "" {
		headers = append(headers, "Message-ID: <"+message.MessageID+"@"+s.host+">")
	}
	raw := strings.Join(headers, "\r\n") + "\r\n\r\n" + body
	deadline := time.Now().UTC().Add(operationTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	operationCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	address := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	connection, err := (&net.Dialer{Timeout: operationTimeout}).DialContext(operationCtx, "tcp", address)
	if err != nil {
		return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("dial SMTP server: %w", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(deadline); err != nil {
		return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("set SMTP deadline: %w", err)
	}
	if s.tlsMode == "implicit" {
		tlsConnection := tls.Client(connection, s.tlsConfig)
		if err := tlsConnection.HandshakeContext(operationCtx); err != nil {
			return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("perform implicit SMTP TLS handshake: %w", err)
		}
		connection = tlsConnection
	}
	client, err := smtp.NewClient(connection, s.host)
	if err != nil {
		return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("create SMTP client: %w", err)
	}
	defer client.Close()
	if s.tlsMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("SMTP server does not advertise required STARTTLS")
		}
		if err := client.StartTLS(s.tlsConfig); err != nil {
			return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("start SMTP TLS: %w", err)
		}
	}
	if s.auth != nil {
		if err := client.Auth(s.auth); err != nil {
			return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("authenticate SMTP client: %w", err)
		}
	}
	if err := client.Mail(s.from); err != nil {
		return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("set SMTP recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("open SMTP message body: %w", err)
	}
	if _, err := writer.Write([]byte(raw)); err != nil {
		_ = writer.Close()
		return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("finalize SMTP message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return notificationsDomain.DeliveryReceipt{}, fmt.Errorf("quit SMTP client: %w", err)
	}
	if err := operationCtx.Err(); err != nil {
		return notificationsDomain.DeliveryReceipt{}, err
	}
	return notificationsDomain.DeliveryReceipt{ProviderMessageID: message.MessageID}, nil
}

var _ notificationsDomain.EmailSender = (*Sender)(nil)
