package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

// EmailSender is the narrow delivery boundary for authentication lifecycle
// mail. Implementations receive a complete URL and must never log credentials.
type EmailSender interface {
	SendVerification(ctx context.Context, to, verificationURL string) error
	SendPasswordReset(ctx context.Context, to, resetURL string) error
}

// DevelopmentEmailSender writes lifecycle URLs to the development log. It must
// never be selected in production.
type DevelopmentEmailSender struct{}

func (DevelopmentEmailSender) SendVerification(_ context.Context, _ string, verificationURL string) error {
	slog.Info("development email verification URL", "url", verificationURL)
	return nil
}

func (DevelopmentEmailSender) SendPasswordReset(_ context.Context, _ string, resetURL string) error {
	slog.Info("development password reset URL", "url", resetURL)
	return nil
}

type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

type SMTPEmailSender struct {
	cfg SMTPConfig
}

func NewSMTPEmailSender(cfg SMTPConfig) (*SMTPEmailSender, error) {
	if strings.TrimSpace(cfg.Host) == "" || strings.TrimSpace(cfg.Port) == "" ||
		strings.TrimSpace(cfg.From) == "" {
		return nil, fmt.Errorf("SMTP host, port, and from address are required")
	}
	return &SMTPEmailSender{cfg: cfg}, nil
}

func (s *SMTPEmailSender) SendVerification(ctx context.Context, to, verificationURL string) error {
	return s.send(ctx, to, "Verify your Alarvest email",
		"Verify your email address to activate your Alarvest account:\r\n\r\n"+verificationURL)
}

func (s *SMTPEmailSender) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	return s.send(ctx, to, "Reset your Alarvest password",
		"Use this one-time link to reset your Alarvest password:\r\n\r\n"+resetURL)
}

func (s *SMTPEmailSender) send(ctx context.Context, to, subject, body string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	addr := s.cfg.Host + ":" + s.cfg.Port
	var auth smtp.Auth
	if s.cfg.Username != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}
	message := []byte("From: " + s.cfg.From + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		body + "\r\n")
	if err := smtp.SendMail(addr, auth, s.cfg.From, []string{to}, message); err != nil {
		return fmt.Errorf("send authentication email: %w", err)
	}
	return nil
}
