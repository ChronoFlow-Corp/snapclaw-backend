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
<html lang="ru">
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
{{if .ShowUnsubscribe}}<p style="margin:10px 0 0 0;font-size:12px;line-height:1.5;color:#9ca3af;">Не хотите получать такие письма? <a href="{{.UnsubscribeURL}}" style="color:#6b7280;">Отписаться</a>.</p>{{end}}
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
		b.WriteString("Отписаться: ")
		b.WriteString(d.UnsubscribeURL)
		b.WriteString("\n")
	}

	return b.String()
}

func (s *Service) renderWelcome(name string) (rendered, error) {
	return s.render(entityWelcomeSubject, layoutData{
		Preheader: "Ваш аккаунт готов — рассказываем, с чего начать.",
		Heading:   "Добро пожаловать в SnapClaw 🐾",
		Paragraphs: []string{
			greeting(name),
			"Рады, что вы с нами! Аккаунт создан и полностью готов к работе.",
			"Вот с чего можно начать:",
		},
		Bullets: []string{
			"Создайте свой первый claw",
			"Подключите его к своему Telegram",
			"Управляйте всем из личного кабинета",
		},
		CTALabel:   "Открыть личный кабинет",
		CTAURL:     s.appURL,
		FooterNote: "Вы получили это письмо, потому что зарегистрировались в SnapClaw. Есть вопросы? Просто ответьте на это письмо.",
	})
}

func (s *Service) renderPremiumGranted(name string) (rendered, error) {
	return s.render(entityPremiumSubject, layoutData{
		Preheader: "Premium уже активен в вашем аккаунте.",
		Heading:   "У вас теперь SnapClaw Premium ✨",
		Paragraphs: []string{
			greeting(name),
			"Отличные новости — Premium уже активен в вашем аккаунте. Ничего оплачивать или настраивать не нужно.",
			"Загляните в личный кабинет, чтобы начать пользоваться всеми возможностями Premium.",
		},
		CTALabel:   "Открыть личный кабинет",
		CTAURL:     s.appURL,
		FooterNote: "Приятного использования! Команда SnapClaw.",
	})
}

func (s *Service) renderTopUp(name, amountValue, amountCurrency string) (rendered, error) {
	amount := strings.TrimSpace(amountValue + " " + amountCurrency)

	return s.render(entityTopUpSubject, layoutData{
		Preheader: "Квитанция о пополнении баланса.",
		Heading:   "Платёж прошёл успешно",
		Paragraphs: []string{
			greeting(name),
			"Мы получили ваш платёж. Детали:",
		},
		Rows: [][2]string{
			{"Сумма", amount},
		},
		CTALabel:   "Посмотреть баланс",
		CTAURL:     s.appURL,
		FooterNote: "Спасибо, что пользуетесь SnapClaw!",
	})
}

func (s *Service) renderSupportReply(name, ticketSubject, replyPreview, ticketURL string) (rendered, error) {
	intro := "Наша команда поддержки ответила на ваше обращение:"
	if strings.TrimSpace(ticketSubject) != "" {
		intro = "Наша команда поддержки ответила на ваше обращение «" + ticketSubject + "»:"
	}

	paragraphs := []string{greeting(name), intro}
	if strings.TrimSpace(replyPreview) != "" {
		paragraphs = append(paragraphs, replyPreview)
	}
	paragraphs = append(paragraphs, "Чтобы продолжить переписку, просто ответьте на это письмо.")

	cta := ""
	if strings.TrimSpace(ticketURL) != "" {
		cta = "Открыть переписку"
	}

	return s.render("Re: "+fallback(ticketSubject, "ваше обращение")+" — поддержка SnapClaw", layoutData{
		Preheader:  "Мы ответили на ваше обращение.",
		Heading:    "Поддержка SnapClaw ответила",
		Paragraphs: paragraphs,
		CTALabel:   cta,
		CTAURL:     ticketURL,
		FooterNote: "— Поддержка SnapClaw",
	})
}

func (s *Service) renderAnnouncement(name, featureName, body, ctaURL, unsubscribeURL string) (rendered, error) {
	cta := ""
	if strings.TrimSpace(ctaURL) != "" {
		cta = "Попробовать"
	}

	return s.render("Новое в SnapClaw: "+featureName, layoutData{
		Preheader:       featureName,
		Heading:         "Новое в SnapClaw: " + featureName,
		Paragraphs:      []string{greeting(name), body},
		CTALabel:        cta,
		CTAURL:          ctaURL,
		FooterNote:      "Вы получаете это письмо, потому что подписались на новости о продукте SnapClaw.",
		ShowUnsubscribe: strings.TrimSpace(unsubscribeURL) != "",
		UnsubscribeURL:  unsubscribeURL,
	})
}

const (
	entityWelcomeSubject = "Добро пожаловать в SnapClaw 🐾"
	entityPremiumSubject = "У вас теперь SnapClaw Premium ✨"
	entityTopUpSubject   = "Платёж в SnapClaw прошёл успешно"
)

func fallback(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}

	return value
}

// greeting builds the salutation. With no name it drops the name entirely
// rather than falling back to a placeholder word.
func greeting(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Привет!"
	}

	return "Привет, " + name + "!"
}
