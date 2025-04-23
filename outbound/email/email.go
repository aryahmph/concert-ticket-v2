package email

import (
	"fmt"
	"github.com/spf13/viper"
	"net/smtp"
	"strings"
)

type EmailOutbound struct {
	Cfg   *viper.Viper
	auth  smtp.Auth
	addr  string
	email string
}

func (out *EmailOutbound) Init() {
	out.email = out.Cfg.GetString("email.user")
	out.addr = fmt.Sprintf("%s:%d", out.Cfg.GetString("email.host"), out.Cfg.GetInt("email.port"))
	out.auth = smtp.CRAMMD5Auth(out.Cfg.GetString("email.user"), out.Cfg.GetString("email.password"))
}

func (out *EmailOutbound) Send(to []string, subject string, body string) error {
	message := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		out.email,
		strings.Join(to, ","),
		subject,
		body,
	))

	err := smtp.SendMail(out.addr, out.auth, out.email, to, message)
	if err != nil {
		return err
	}

	return nil
}
