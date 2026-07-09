package mail

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
)

type Mailer struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

func (m *Mailer) Enabled() bool {
	return m.Host != "" && m.From != ""
}

func (m *Mailer) Send(to, subject, body string) error {
	if !m.Enabled() {
		return nil
	}

	addr := net.JoinHostPort(m.Host, m.Port)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n",
		m.From, to, subject, body)

	var auth smtp.Auth
	if m.Username != "" {
		auth = smtp.PlainAuth("", m.Username, m.Password, m.Host)
	}

	if m.Port == "465" {
		return sendImplicitTLS(addr, m.Host, auth, m.From, to, msg)
	}
	return smtp.SendMail(addr, auth, m.From, []string{to}, []byte(msg))
}

func sendImplicitTLS(addr, host string, auth smtp.Auth, from, to, msg string) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}
