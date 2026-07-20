package email

import (
	"bytes"
	"strings"
)

// layoutData holds everything the shared email layout needs. All fields are
// rendered through html/template, so any dynamic value is auto-escaped.
type layoutData struct {
	FromName        string
	Preheader       string
	Heading         string
	Paragraphs      []string
	Bullets         []string
	Rows            [][2]string
	CTALabel        string
	CTAURL          string
	FooterNote      string
	ShowUnsubscribe bool
	UnsubscribeURL  string
}

const layoutHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Heading}}</title>
</head>
<body style="margin:0;padding:0;background:#f4f4f5;">
<span style="display:none;max-height:0;overflow:hidden;opacity:0;">{{.Preheader}}</span>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f4f5;padding:24px 0;">
<tr><td align="center">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border-radius:12px;overflow:hidden;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<tr><td style="padding:28px 32px 8px 32px;">
<div style="font-size:18px;font-weight:700;color:#111827;">{{.FromName}}</div>
</td></tr>
<tr><td style="padding:8px 32px 0 32px;">
<h1 style="margin:0 0 12px 0;font-size:22px;line-height:1.3;color:#111827;">{{.Heading}}</h1>
{{range .Paragraphs}}<p style="margin:0 0 14px 0;font-size:15px;line-height:1.6;color:#374151;">{{.}}</p>{{end}}
{{if .Bullets}}<ul style="margin:0 0 14px 0;padding-left:20px;font-size:15px;line-height:1.6;color:#374151;">{{range .Bullets}}<li style="margin:0 0 6px 0;">{{.}}</li>{{end}}</ul>{{end}}
{{if .Rows}}<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:0 0 14px 0;font-size:15px;color:#374151;">{{range .Rows}}<tr><td style="padding:6px 0;color:#6b7280;">{{index . 0}}</td><td align="right" style="padding:6px 0;font-weight:600;color:#111827;">{{index . 1}}</td></tr>{{end}}</table>{{end}}
{{if .CTALabel}}<table role="presentation" cellpadding="0" cellspacing="0" style="margin:6px 0 18px 0;"><tr><td style="border-radius:8px;background:#4f46e5;"><a href="{{.CTAURL}}" style="display:inline-block;padding:12px 22px;font-size:15px;font-weight:600;color:#ffffff;text-decoration:none;border-radius:8px;">{{.CTALabel}}</a></td></tr></table>{{end}}
</td></tr>
<tr><td style="padding:8px 32px 28px 32px;border-top:1px solid #f0f0f1;">
{{if .FooterNote}}<p style="margin:14px 0 0 0;font-size:12px;line-height:1.5;color:#9ca3af;">{{.FooterNote}}</p>{{end}}
{{if .ShowUnsubscribe}}<p style="margin:10px 0 0 0;font-size:12px;line-height:1.5;color:#9ca3af;">Don't want these updates? <a href="{{.UnsubscribeURL}}" style="color:#6b7280;">Unsubscribe</a>.</p>{{end}}
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`

func (s *Service) renderHTML(d layoutData) (string, error) {
	var buf bytes.Buffer
	if err := s.tmpl.Execute(&buf, d); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func layoutText(d layoutData) string {
	var b strings.Builder

	b.WriteString(d.Heading)
	b.WriteString("\n\n")

	for _, p := range d.Paragraphs {
		b.WriteString(p)
		b.WriteString("\n\n")
	}

	for _, item := range d.Bullets {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	if len(d.Bullets) > 0 {
		b.WriteString("\n")
	}

	for _, row := range d.Rows {
		b.WriteString(row[0])
		b.WriteString(": ")
		b.WriteString(row[1])
		b.WriteString("\n")
	}
	if len(d.Rows) > 0 {
		b.WriteString("\n")
	}

	if d.CTALabel != "" {
		b.WriteString(d.CTALabel)
		b.WriteString(": ")
		b.WriteString(d.CTAURL)
		b.WriteString("\n\n")
	}

	if d.FooterNote != "" {
		b.WriteString(d.FooterNote)
		b.WriteString("\n")
	}

	if d.ShowUnsubscribe {
		b.WriteString("Unsubscribe: ")
		b.WriteString(d.UnsubscribeURL)
		b.WriteString("\n")
	}

	return b.String()
}

func (s *Service) renderWelcome(name string) (rendered, error) {
	return s.render(entityWelcomeSubject, layoutData{
		Preheader: "Your account is ready — here's how to get started.",
		Heading:   "Welcome to SnapClaw 🐾",
		Paragraphs: []string{
			"Hi " + name + ",",
			"Welcome to SnapClaw! Your account is all set up and ready to go.",
			"Here's what you can do right now:",
		},
		Bullets: []string{
			"[Create your first claw] — [one line on what a claw does]",
			"[Connect it to Telegram] — [one line]",
			"[Explore the dashboard] — manage everything in one place",
		},
		CTALabel:   "Open my dashboard",
		CTAURL:     s.appURL,
		FooterNote: "You're receiving this because you signed up for SnapClaw. Questions? Just reply to this email.",
	})
}

func (s *Service) renderPremiumGranted(name string) (rendered, error) {
	return s.render(entityPremiumSubject, layoutData{
		Preheader: "Premium is now active on your account.",
		Heading:   "You've got SnapClaw Premium ✨",
		Paragraphs: []string{
			"Hi " + name + ",",
			"Good news — Premium is now active on your account. Nothing to pay, nothing to do.",
			"What's unlocked:",
		},
		Bullets: []string{
			"[Higher limits / more claws]",
			"[Priority processing]",
			"[Premium-only feature]",
		},
		CTALabel:   "See what's new",
		CTAURL:     s.appURL,
		FooterNote: "Enjoy — the SnapClaw team.",
	})
}

func (s *Service) renderTopUp(name, amountValue, amountCurrency string) (rendered, error) {
	amount := strings.TrimSpace(amountValue + " " + amountCurrency)

	return s.render(entityTopUpSubject, layoutData{
		Preheader: "Receipt for your recent top-up.",
		Heading:   "Your SnapClaw payment was successful",
		Paragraphs: []string{
			"Hi " + name + ",",
			"We've received your payment. Here are the details:",
		},
		Rows: [][2]string{
			{"Amount", amount},
		},
		CTALabel:   "View my balance",
		CTAURL:     s.appURL,
		FooterNote: "Thanks for using SnapClaw!",
	})
}

func (s *Service) renderSupportReply(name, ticketSubject, replyPreview, ticketURL string) (rendered, error) {
	intro := "Our support team just replied to your request:"
	if strings.TrimSpace(ticketSubject) != "" {
		intro = "Our support team just replied to your request \"" + ticketSubject + "\":"
	}

	paragraphs := []string{"Hi " + name + ",", intro}
	if strings.TrimSpace(replyPreview) != "" {
		paragraphs = append(paragraphs, replyPreview)
	}
	paragraphs = append(paragraphs, "You can reply directly to this email to continue the conversation.")

	cta := ""
	if strings.TrimSpace(ticketURL) != "" {
		cta = "View the full conversation"
	}

	return s.render("Re: "+fallback(ticketSubject, "your request")+" — SnapClaw Support", layoutData{
		Preheader:  "We've replied to your request.",
		Heading:    "SnapClaw Support replied",
		Paragraphs: paragraphs,
		CTALabel:   cta,
		CTAURL:     ticketURL,
		FooterNote: "— SnapClaw Support",
	})
}

func (s *Service) renderAnnouncement(name, featureName, body, ctaURL, unsubscribeURL string) (rendered, error) {
	cta := ""
	if strings.TrimSpace(ctaURL) != "" {
		cta = "Try it now"
	}

	return s.render("New in SnapClaw: "+featureName, layoutData{
		Preheader:       featureName,
		Heading:         "New in SnapClaw: " + featureName,
		Paragraphs:      []string{"Hi " + name + ",", body},
		CTALabel:        cta,
		CTAURL:          ctaURL,
		FooterNote:      "You're getting this because you opted in to product updates from SnapClaw.",
		ShowUnsubscribe: strings.TrimSpace(unsubscribeURL) != "",
		UnsubscribeURL:  unsubscribeURL,
	})
}

const (
	entityWelcomeSubject = "Welcome to SnapClaw 🐾"
	entityPremiumSubject = "You've got SnapClaw Premium ✨"
	entityTopUpSubject   = "Your SnapClaw payment was successful"
)

func fallback(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}

	return value
}
