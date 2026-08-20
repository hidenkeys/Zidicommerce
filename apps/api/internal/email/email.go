package email

import (
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

type Message struct {
	To      string
	Subject string
	Text    string
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
	s.logger.Info("email queued in development mode", "to", message.To, "subject", message.Subject, "from", s.from)
	return nil
}

type SMTPSender struct {
	host     string
	port     string
	username string
	password string
	from     string
}

func NewSMTPSender(host, port, username, password, from string) *SMTPSender {
	return &SMTPSender{host: host, port: port, username: username, password: password, from: from}
}

func (s *SMTPSender) Send(message Message) error {
	if s.host == "" || s.port == "" || s.username == "" || s.password == "" || s.from == "" {
		return fmt.Errorf("smtp sender is not fully configured")
	}
	to := strings.TrimSpace(message.To)
	if to == "" {
		return fmt.Errorf("email recipient is required")
	}
	body := "From: " + s.from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + message.Subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		message.Text
	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	return smtp.SendMail(s.host+":"+s.port, auth, s.from, []string{to}, []byte(body))
}
