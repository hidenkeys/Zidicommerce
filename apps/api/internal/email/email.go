package email

import (
	"fmt"
	"log/slog"
	"strings"
)

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

type Sender interface {
	Send(message Message) error
}

type LogSender struct {
	logger *slog.Logger
	from   string
}

func NewLogSender(logger *slog.Logger, from string) *LogSender {
	return &LogSender{logger: logger, from: from}
}

func (s *LogSender) Send(message Message) error {
	if s.logger != nil {
		s.logger.Info("email queued in development mode", "to_domain", recipientDomain(message.To), "subject", message.Subject, "from", s.from)
	}
	return nil
}

func NewSender(mode, host, port, username, password, from, fromName, tlsMode string, logger *slog.Logger) (Sender, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", "log", "dev", "development":
		return NewLogSender(logger, from), nil
	case "smtp":
		sender := NewSMTPSender(host, port, username, password, from, fromName, tlsMode)
		if err := sender.Validate(); err != nil {
			return nil, err
		}
		return sender, nil
	default:
		return nil, fmt.Errorf("unsupported email mode %q", mode)
	}
}

func MaskRecipient(address string) string {
	address = strings.TrimSpace(strings.ToLower(address))
	at := strings.LastIndex(address, "@")
	if at <= 0 || at == len(address)-1 {
		return "***"
	}
	local := address[:at]
	domain := address[at+1:]
	visible := string(local[0])
	return visible + "***@" + domain
}

func recipientDomain(address string) string {
	at := strings.LastIndex(address, "@")
	if at < 0 || at == len(address)-1 {
		return "unknown"
	}
	return strings.ToLower(address[at+1:])
}

func SanitizeSMTPError(err error) string {
	if err == nil {
		return ""
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "535") || strings.Contains(lower, "534") || strings.Contains(lower, "auth"):
		return "The email service could not sign in. Check the SMTP username and password."
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "i/o timeout"):
		return "The email service timed out. Try again in a moment."
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") || strings.Contains(lower, "dial"):
		return "Could not reach the email service."
	case strings.Contains(lower, "tls") || strings.Contains(lower, "certificate"):
		return "Could not establish a secure connection to the email service."
	case strings.Contains(lower, "not fully configured"):
		return "Email sending is not configured."
	default:
		return "The invitation email could not be delivered."
	}
}
