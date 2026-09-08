package mail

import (
	"fmt"
	"net/smtp"

	"walletx-be/configs"
)

type EmailService struct {
	config configs.SMTPConfig
}

func NewEmailService(cfg configs.SMTPConfig) *EmailService {
	return &EmailService{
		config: cfg,
	}
}

func (s *EmailService) SendVerificationEmail(toEmail string, verificationLink string) error {
	from := s.config.User
	password := s.config.Password
	smtpHost := s.config.Host
	smtpPort := s.config.Port

	// Create authentication
	auth := smtp.PlainAuth("", from, password, smtpHost)

	// Setup Headers
	headers := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"

	subject := "Subject: WalletX - Telegram Account Verification\n"

	// Create HTML body
	htmlBody := fmt.Sprintf(`
		<html>
		<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
			<h2>Verifikasi Akun Telegram WalletX</h2>
			<p>Halo,</p>
			<p>Anda menerima email ini karena ada permintaan untuk menghubungkan akun Telegram dengan WalletX Anda.</p>
			<p>Silakan klik tautan di bawah ini untuk mengonfirmasi:</p>
			<p style="margin: 20px 0;">
				<a href="%s" style="background-color: #4CAF50; color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px;">Konfirmasi Telegram</a>
			</p>
			<p>Atau copy-paste URL berikut ke browser Anda:</p>
			<p><small><a href="%s">%s</a></small></p>
			<p>Tautan ini hanya berlaku selama 15 menit.</p>
			<br>
			<p>Terima kasih,<br>Tim WalletX</p>
		</body>
		</html>
	`, verificationLink, verificationLink, verificationLink)

	msg := []byte(subject + headers + htmlBody)

	// Send email
	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, from, []string{toEmail}, msg)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}
