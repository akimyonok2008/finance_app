package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	emailOutboxLeaseTTL   = 5 * time.Minute
	emailOutboxRetryDelay = 15 * time.Second
	emailOutboxBatchSize  = 20
)

// ProcessEmailOutboxOnce claims a bounded batch and attempts delivery. Failed
// messages are released with a retry time; a failure never deletes the durable
// intent or invalidates the associated verification token.
func (s *Service) ProcessEmailOutboxOnce(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	messages, err := s.repo.ClaimEmailOutbox(ctx, emailOutboxBatchSize, now)
	if err != nil {
		return 0, err
	}
	delivered := 0
	var deliveryErrors []error
	for _, message := range messages {
		if err := s.deliverClaimedEmail(ctx, message, now); err != nil {
			deliveryErrors = append(deliveryErrors, err)
		} else {
			delivered++
		}
	}
	return delivered, errors.Join(deliveryErrors...)
}

func (s *Service) deliverEmailOutboxByID(ctx context.Context, id string) error {
	now := time.Now().UTC()
	message, claimed, err := s.repo.ClaimEmailOutboxByID(ctx, id, now)
	if err != nil || !claimed {
		return err
	}
	return s.deliverClaimedEmail(ctx, message, now)
}

func (s *Service) deliverClaimedEmail(
	ctx context.Context,
	message EmailOutboxMessage,
	now time.Time,
) error {
	var sendErr error
	switch message.Kind {
	case EmailKindVerification:
		sendErr = s.emailSender.SendVerification(
			ctx, message.Recipient, message.VerificationURL,
		)
	default:
		sendErr = fmt.Errorf("unsupported auth email kind %q", message.Kind)
	}
	if sendErr != nil {
		markErr := s.repo.MarkEmailOutboxFailed(
			ctx, message.ID, sendErr.Error(), now.Add(emailOutboxRetryDelay),
		)
		return errors.Join(
			fmt.Errorf("deliver auth email %s: %w", message.ID, sendErr),
			markErr,
		)
	}
	if err := s.repo.MarkEmailOutboxDelivered(ctx, message.ID, time.Now().UTC()); err != nil {
		return fmt.Errorf("mark auth email %s delivered: %w", message.ID, err)
	}
	return nil
}

// StartEmailOutboxProcessor runs durable email delivery independently of the
// optional analytics/background jobs. PostgreSQL registration relies on this
// processor even when ENABLE_BACKGROUND_WORKERS is false.
func (s *Service) StartEmailOutboxProcessor(ctx context.Context, interval time.Duration) <-chan struct{} {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		run := func() {
			delivered, err := s.ProcessEmailOutboxOnce(ctx)
			if err != nil && ctx.Err() == nil {
				slog.Warn("authentication email outbox pass failed", "error", err)
			}
			if delivered > 0 {
				slog.Info("authentication emails delivered", "count", delivered)
			}
		}
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
	return done
}
