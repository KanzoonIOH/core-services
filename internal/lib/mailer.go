package lib

import (
	"context"
	"fmt"

	"github.com/resend/resend-go/v3"
)

type Mailer struct {
	client *resend.Client
	from   string
	appUrl string
}

func NewMailer(apiKey, from string, appUrl string) *Mailer {
	return &Mailer{
		client: resend.NewClient(apiKey),
		from:   from,
		appUrl: appUrl,
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

func (m *Mailer) IssueURL(token string) string {
	url := fmt.Sprintf("%s?token=%s", m.appUrl, token)
	return url
}

// IssuePathURL builds a link to a specific frontend path (e.g. "/invite")
// carrying the token as the "key" query param.
func (m *Mailer) IssuePathURL(path, token string) string {
	return fmt.Sprintf("%s%s?key=%s", m.appUrl, path, token)
}
