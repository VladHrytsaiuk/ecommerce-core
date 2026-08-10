package smtp

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	notificationsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
)

func TestNewRejectsIncompleteConfiguration(t *testing.T) {
	if _, err := New(Config{Host: "smtp.example.com", Port: 587, From: "no-reply@example.com", Username: "user"}); err == nil {
		t.Fatal("New() error = nil, want missing password error")
	}
}

func TestSendHonorsContextDeadlineWhileSMTPGreetingStalls(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	sender, err := New(Config{Host: "127.0.0.1", Port: port, From: "no-reply@example.com"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = sender.Send(ctx, notificationsDomain.EmailMessage{MessageID: "receipt-1", To: "buyer@example.com", Subject: "Receipt", Text: "Paid"})
	if err == nil {
		t.Fatal("Send() error = nil, want greeting deadline")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Send() elapsed %s, deadline was ignored", elapsed)
	}
	select {
	case connection := <-accepted:
		_ = connection.Close()
	default:
	}
}

func TestSendFailsClosedWhenServerDoesNotOfferSTARTTLS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	commands := make(chan string, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		_, _ = fmt.Fprint(connection, "220 smtp.test ESMTP\r\n")
		buffer := make([]byte, 512)
		read, _ := connection.Read(buffer)
		commands <- string(buffer[:read])
		_, _ = fmt.Fprint(connection, "250 smtp.test\r\n")
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	sender, err := New(Config{Host: "127.0.0.1", Port: port, From: "no-reply@example.com", Username: "user", Password: "password"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = sender.Send(context.Background(), notificationsDomain.EmailMessage{MessageID: "receipt-2", To: "buyer@example.com", Subject: "Receipt", Text: "Paid"})
	if err == nil || !strings.Contains(err.Error(), "required STARTTLS") {
		t.Fatalf("Send() error = %v, want required STARTTLS failure", err)
	}
	if command := <-commands; !strings.HasPrefix(command, "EHLO") || strings.Contains(command, "AUTH") {
		t.Fatalf("SMTP commands before TLS = %q, want EHLO only", command)
	}
}
