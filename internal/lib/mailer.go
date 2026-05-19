package lib

import (
	"context"
	"fmt"

	"github.com/resend/resend-go/v3"
)

type Mailer struct {
	client *resend.Client
	from   string
}

func NewMailer(apiKey, from string) *Mailer {
	return &Mailer{
		client: resend.NewClient(apiKey),
		from:   from,
	}
}

func (m *Mailer) Send(ctx context.Context, to, subject, text string) error {
	params := &resend.SendEmailRequest{
		From:    m.from,
		To:      []string{to},
		Subject: subject,
		Text:    text,
	}

	_, err := m.client.Emails.SendWithContext(ctx, params)
	if err != nil {
		return fmt.Errorf("mailer send: %w", err)
	}
	return nil
}
