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
	BadgeLabel      string
	BadgeTone       string // "teal" (default) or "green"
	Heading         string
	Paragraphs      []string
	Quote           string
	Bullets         []string
	Rows            [][2]string
	Highlight       bool // renders Rows inside a soft success box instead of a plain list
	CTALabel        string
	CTAURL          string
	FooterNote      string
	ShowUnsubscribe bool
	UnsubscribeURL  string
}

// Colors and radii below mirror the marketing site (frontend/src/components/landing):
// #19b298/#16a087 CTA teal, #34c759 success green, #F7F3EF warm background, #171717
// near-black logo mark, rounded-[16px] buttons and Helvetica Neue as the rendered font.
const layoutHTML = `<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Heading}}</title>
</head>
<body style="margin:0;padding:0;background:#F7F3EF;">
<span style="display:none;max-height:0;overflow:hidden;opacity:0;">{{.Preheader}}</span>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#F7F3EF;padding:32px 0;">
<tr><td align="center">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border-radius:20px;overflow:hidden;font-family:'Helvetica Neue',Helvetica,Arial,sans-serif;">
<tr><td style="padding:28px 32px 4px 32px;">
<table role="presentation" cellpadding="0" cellspacing="0"><tr>
<td width="32" style="width:32px;height:32px;background:#171717;border-radius:9px;text-align:center;vertical-align:middle;font-size:15px;line-height:32px;">🐾</td>
<td style="padding-left:10px;font-size:16px;font-weight:700;color:#171717;vertical-align:middle;">{{.FromName}}</td>
</tr></table>
</td></tr>
{{if .BadgeLabel}}<tr><td style="padding:16px 32px 0 32px;">
<span style="display:inline-block;padding:4px 12px;border-radius:999px;font-size:12px;font-weight:600;{{if eq .BadgeTone "green"}}background:rgba(52,199,89,0.15);color:#0b7a34;{{else}}background:rgba(25,178,152,0.12);color:#0f7a68;{{end}}">{{.BadgeLabel}}</span>
</td></tr>{{end}}
<tr><td style="padding:16px 32px 0 32px;">
<h1 style="margin:0 0 12px 0;font-size:22px;line-height:1.3;color:#171717;">{{.Heading}}</h1>
{{range .Paragraphs}}<p style="margin:0 0 14px 0;font-size:15px;line-height:1.6;color:#44403c;">{{.}}</p>{{end}}
{{if .Quote}}<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:0 0 16px 0;"><tr><td style="border-left:3px solid #19b298;background:#f8f7f5;border-radius:0 10px 10px 0;padding:12px 16px;font-size:15px;line-height:1.6;color:#44403c;">{{.Quote}}</td></tr></table>{{end}}
{{if .Bullets}}<ul style="margin:0 0 16px 0;padding-left:20px;font-size:15px;line-height:1.6;color:#44403c;">{{range .Bullets}}<li style="margin:0 0 6px 0;">{{.}}</li>{{end}}</ul>{{end}}
{{if .Rows}}{{if .Highlight}}<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:0 0 16px 0;background:#f2fbf6;border:1px solid rgba(52,199,89,0.35);border-radius:14px;"><tr><td style="padding:14px 16px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="font-size:15px;">{{range .Rows}}<tr><td style="padding:6px 0;color:#78716c;">{{index . 0}}</td><td align="right" style="padding:6px 0;font-weight:700;color:#0b7a34;">{{index . 1}}</td></tr>{{end}}</table>
</td></tr></table>{{else}}<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:0 0 16px 0;font-size:15px;">{{range .Rows}}<tr><td style="padding:6px 0;color:#78716c;">{{index . 0}}</td><td align="right" style="padding:6px 0;font-weight:700;color:#171717;">{{index . 1}}</td></tr>{{end}}</table>{{end}}{{end}}
{{if .CTALabel}}<table role="presentation" cellpadding="0" cellspacing="0" style="margin:4px 0 20px 0;"><tr><td style="border-radius:16px;background:#19b298;"><a href="{{.CTAURL}}" style="display:inline-block;padding:14px 26px;font-size:15px;font-weight:600;color:#ffffff;text-decoration:none;border-radius:16px;">{{.CTALabel}}</a></td></tr></table>{{end}}
</td></tr>
<tr><td style="padding:8px 32px 28px 32px;border-top:1px solid #ece7e1;">
{{if .FooterNote}}<p style="margin:14px 0 0 0;font-size:12px;line-height:1.5;color:#a8a29e;">{{.FooterNote}}</p>{{end}}
{{if .ShowUnsubscribe}}<p style="margin:10px 0 0 0;font-size:12px;line-height:1.5;color:#a8a29e;">Не хотите получать такие письма? <a href="{{.UnsubscribeURL}}" style="color:#78716c;">Отписаться</a>.</p>{{end}}
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

	if d.BadgeLabel != "" {
		b.WriteString("[" + d.BadgeLabel + "]\n")
	}

	b.WriteString(d.Heading)
	b.WriteString("\n\n")

	for _, p := range d.Paragraphs {
		b.WriteString(p)
		b.WriteString("\n\n")
	}

	if d.Quote != "" {
		b.WriteString("> ")
		b.WriteString(d.Quote)
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
		Preheader:  "Premium уже активен в вашем аккаунте.",
		BadgeLabel: "✨ Premium",
		BadgeTone:  "teal",
		Heading:    "У вас теперь SnapClaw Premium",
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
		Preheader:  "Квитанция о пополнении баланса.",
		BadgeLabel: "Оплачено",
		BadgeTone:  "green",
		Heading:    "Платёж прошёл успешно",
		Paragraphs: []string{
			greeting(name),
			"Мы получили ваш платёж. Детали:",
		},
		Rows: [][2]string{
			{"Сумма", amount},
		},
		Highlight:  true,
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

	cta := ""
	if strings.TrimSpace(ticketURL) != "" {
		cta = "Открыть переписку"
	}

	return s.render("Re: "+fallback(ticketSubject, "ваше обращение")+" — поддержка SnapClaw", layoutData{
		Preheader:  "Мы ответили на ваше обращение.",
		Heading:    "Поддержка SnapClaw ответила",
		Paragraphs: []string{greeting(name), intro},
		Quote:      replyPreview,
		CTALabel:   cta,
		CTAURL:     ticketURL,
		FooterNote: "Чтобы продолжить переписку, просто ответьте на это письмо. — Поддержка SnapClaw",
	})
}

func (s *Service) renderAnnouncement(name, featureName, body, ctaURL, unsubscribeURL string) (rendered, error) {
	cta := ""
	if strings.TrimSpace(ctaURL) != "" {
		cta = "Попробовать"
	}

	return s.render("Новое в SnapClaw: "+featureName, layoutData{
		Preheader:       featureName,
		BadgeLabel:      "Новое",
		BadgeTone:       "teal",
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
