package email

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"
)

func TestMaskRecipient(t *testing.T) {
	if got := MaskRecipient("john@example.com"); got != "j***@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestSanitizeSMTPErrorHidesRawDetails(t *testing.T) {
	err := fmt.Errorf("535 Authentication failed for secret-password")
	got := SanitizeSMTPError(err)
	if strings.Contains(got, "secret-password") || strings.Contains(got, "535") {
		t.Fatalf("sanitized error leaked implementation details: %q", got)
	}
}

func TestNewSenderLogMode(t *testing.T) {
	sender, err := NewSender("log", "", "", "", "", "noreply@example.com", "ZidiCommerce", "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sender.(*LogSender); !ok {
		t.Fatalf("expected log sender, got %T", sender)
	}
}

func TestNewSenderSMTPRequiresConfig(t *testing.T) {
	_, err := NewSender("smtp", "", "587", "", "", "noreply@example.com", "ZidiCommerce", "starttls", nil)
	if err == nil {
		t.Fatal("expected missing SMTP configuration to fail")
	}
}

func TestNewSenderSMTPReady(t *testing.T) {
	sender, err := NewSender("smtp", "smtp.zoho.com", "587", "admin@example.com", "app-password", "admin@example.com", "ZidiCommerce", "starttls", nil)
	if err != nil {
		t.Fatal(err)
	}
	smtpSender, ok := sender.(*SMTPSender)
	if !ok {
		t.Fatalf("expected smtp sender, got %T", sender)
	}
	if smtpSender.password == "" || smtpSender.host != "smtp.zoho.com" {
		t.Fatal("smtp sender was not initialized")
	}
}

func TestInvitationURLAndTemplate(t *testing.T) {
	url := InvitationURL("https://admin.example.com", "abc123")
	if url != "https://admin.example.com/invitations/accept?token=abc123" {
		t.Fatalf("unexpected url %q", url)
	}
	message := BuildInvitation(InvitationContent{
		BusinessName:   "Bing Chun",
		InviterName:    "Ada Merchant",
		RoleLabel:      "Merchant Admin",
		StoreNames:     []string{"Jara"},
		AcceptURL:      url,
		ExpiresAt:      time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC),
		RecipientEmail: "new@example.com",
	})
	if !strings.Contains(message.Subject, "Bing Chun") {
		t.Fatalf("subject missing business name: %q", message.Subject)
	}
	for _, body := range []string{message.Text, message.HTML} {
		if !strings.Contains(body, "Ada Merchant") || !strings.Contains(body, "Merchant Admin") || !strings.Contains(body, "Jara") || !strings.Contains(body, url) {
			t.Fatalf("template missing expected content: %s", body)
		}
		if strings.Contains(strings.ToLower(body), "uuid") || strings.Contains(body, "organization_id") {
			t.Fatal("template leaked internal identifiers")
		}
	}
	if !strings.Contains(message.HTML, "Accept invitation") {
		t.Fatal("html email should include accept button")
	}
}

func TestSMTPSenderMissingConfig(t *testing.T) {
	sender := NewSMTPSender("", "587", "user@example.com", "secret-password", "user@example.com", "ZidiCommerce", "starttls")
	err := sender.Send(Message{To: "to@example.com", Subject: "Hi", Text: "Hello"})
	if err == nil {
		t.Fatal("expected configuration error")
	}
	if strings.Contains(err.Error(), "secret-password") {
		t.Fatal("smtp error leaked password")
	}
}

func TestSMTPSenderRequiresSTARTTLS(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("220 mock ESMTP\r\n"))
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("250-mock\r\n250 AUTH PLAIN\r\n"))
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	sender := NewSMTPSender("127.0.0.1", port, "user@example.com", "secret-password", "user@example.com", "ZidiCommerce", "starttls")
	err = sender.Send(Message{To: "to@example.com", Subject: "Hi", Text: "Hello"})
	<-done
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "starttls") {
		t.Fatalf("expected STARTTLS error, got %v", err)
	}
	if strings.Contains(err.Error(), "secret-password") {
		t.Fatal("smtp error leaked password")
	}
}
