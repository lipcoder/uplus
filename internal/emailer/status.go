package mailer

import (
	"net/smtp"
)

type Mailer struct {
	from     string
	password string
}

func New(from, password string) *Mailer {
	return &Mailer{
		from:     from,
		password: password,
	}
}

func (m *Mailer) Send(to, text string) error {
	const host = "smtp.qq.com"

	auth := smtp.PlainAuth(
		"",
		m.from,
		m.password,
		host,
	)

	msg := []byte(
		"From: " + m.from + "\r\n" +
			"To: " + to + "\r\n" +
			"Subject: " + text + "\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" +
			text,
	)

	return smtp.SendMail(
		host+":587",
		auth,
		m.from,
		[]string{to},
		msg,
	)
}