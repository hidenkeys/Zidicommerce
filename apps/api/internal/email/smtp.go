package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type SMTPSender struct {
	host     string
	port     string
	username string
	password string
	from     string
	fromName string
	tlsMode  string
}

func NewSMTPSender(host, port, username, password, from, fromName, tlsMode string) *SMTPSender {
	return &SMTPSender{
		host:     strings.TrimSpace(host),
		port:     strings.TrimSpace(port),
		username: strings.TrimSpace(username),
		password: password,
		from:     strings.TrimSpace(from),
		fromName: strings.TrimSpace(fromName),
		tlsMode:  strings.ToLower(strings.TrimSpace(tlsMode)),
	}
}

func (s *SMTPSender) Validate() error {
	if s.host == "" || s.port == "" || s.username == "" || strings.TrimSpace(s.password) == "" || s.from == "" {
		return fmt.Errorf("smtp sender is not fully configured")
	}
	return nil
}

func (s *SMTPSender) Send(message Message) error {
	if err := s.Validate(); err != nil {
		return err
	}
	to := strings.TrimSpace(message.To)
	if to == "" {
		return fmt.Errorf("email recipient is required")
	}
	addr := net.JoinHostPort(s.host, s.port)
	tlsConfig := &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}
	dialer := &net.Dialer{Timeout: 15 * time.Second}

	var conn net.Conn
	var err error
	if s.useImplicitTLS() {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return err
	}
	defer client.Close()

	if !s.useImplicitTLS() {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}

	if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
		return err
	}
	if err := client.Mail(s.from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(s.compose(message, to))); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (s *SMTPSender) useImplicitTLS() bool {
	return s.port == "465" || s.tlsMode == "ssl" || s.tlsMode == "tls"
}

func (s *SMTPSender) compose(message Message, to string) string {
	fromHeader := s.from
	if s.fromName != "" {
		fromHeader = fmt.Sprintf("%q <%s>", s.fromName, s.from)
	}
	headers := []string{
		"From: " + fromHeader,
		"To: " + to,
		"Subject: " + strings.ReplaceAll(message.Subject, "\n", " "),
		"MIME-Version: 1.0",
	}
	text := message.Text
	if strings.TrimSpace(text) == "" {
		text = stripHTML(message.HTML)
	}
	html := message.HTML
	if strings.TrimSpace(html) == "" {
		headers = append(headers, "Content-Type: text/plain; charset=UTF-8", "", text)
		return strings.Join(headers, "\r\n")
	}
	boundary := "zidicommerce-alt"
	headers = append(headers, "Content-Type: multipart/alternative; boundary="+boundary, "")
	body := []string{
		"--" + boundary,
		"Content-Type: text/plain; charset=UTF-8",
		"",
		text,
		"--" + boundary,
		"Content-Type: text/html; charset=UTF-8",
		"",
		html,
		"--" + boundary + "--",
		"",
	}
	return strings.Join(headers, "\r\n") + "\r\n" + strings.Join(body, "\r\n")
}

func stripHTML(value string) string {
	replaced := strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n\n", "</div>", "\n").Replace(value)
	var builder strings.Builder
	inTag := false
	for _, r := range replaced {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				builder.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(builder.String())
}
