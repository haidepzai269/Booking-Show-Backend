package service

import (
	"fmt"
	"net/smtp"

	"github.com/booking-show/booking-show-api/config"
)

type EmailService struct {
	Cfg *config.Config
}

func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{Cfg: cfg}
}

func (s *EmailService) SendMagicLink(email, token string) error {
	magicLink := fmt.Sprintf("%s/magic-login?token=%s", s.Cfg.FrontendURL, token)
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", s.Cfg.FrontendURL, token)

	subject := "Subject: [Booking-show] Yêu cầu Đăng nhập & Đổi mật khẩu\n"
	mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
	body := fmt.Sprintf(`
		<html>
		<body style="font-family: sans-serif; line-height: 1.6; color: #333;">
			<div style="max-width: 600px; margin: 0 auto; padding: 20px; border: 1px solid #eee; border-radius: 10px;">
				<h2 style="color: #e50914;">Chào bạn!</h2>
				<p>Bạn đã yêu cầu truy cập vào <strong>Booking-show</strong>.</p>
				
				<div style="margin: 30px 0;">
					<p><strong>Cách 1: Đăng nhập nhanh (không cần mật khẩu)</strong></p>
					<a href="%s" style="display:inline-block;padding:12px 24px;background-color:#e50914;color:white;text-decoration:none;border-radius:5px;font-weight:bold;">Đăng nhập ngay</a>
				</div>

				<div style="margin: 30px 0; padding-top: 20px; border-top: 1px solid #eee;">
					<p><strong>Cách 2: Đặt lại mật khẩu mới</strong></p>
					<p>Nếu bạn muốn đổi mật khẩu để sử dụng cho lần sau, hãy nhấn vào nút dưới đây:</p>
					<a href="%s" style="display:inline-block;padding:12px 24px;background-color:#333;color:white;text-decoration:none;border-radius:5px;font-weight:bold;">Đặt lại mật khẩu</a>
				</div>

				<p style="font-size: 12px; color: #777; margin-top: 40px;">
					Các liên kết trên sẽ hết hạn sau 15 phút.<br/>
					Nếu bạn không thực hiện yêu cầu này, vui lòng bỏ qua email này.
				</p>
				<p>Trân trọng,<br/><strong>Booking-show Team</strong></p>
			</div>
		</body>
		</html>
	`, magicLink, resetLink)

	msg := []byte(subject + mime + body)
	addr := fmt.Sprintf("%s:%s", s.Cfg.SMTPHost, s.Cfg.SMTPPort)

	auth := smtp.PlainAuth("", s.Cfg.SMTPUser, s.Cfg.SMTPPass, s.Cfg.SMTPHost)

	err := smtp.SendMail(addr, auth, s.Cfg.SMTPUser, []string{email}, msg)
	if err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}

	return nil
}
