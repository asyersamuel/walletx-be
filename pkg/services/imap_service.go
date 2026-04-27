package services

import (
	"context"
	"io"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/sirupsen/logrus"
)

type IMAPService struct {
	Server    string
	Email     string
	Password  string
	TxService TransactionService
}

func NewIMAPService(email, password string, txService TransactionService) *IMAPService {
	return &IMAPService{
		Server:    "imap.gmail.com:993",
		Email:     email,
		Password:  password,
		TxService: txService,
	}
}

func (s *IMAPService) ProcessUnseenEmails() error {
	logrus.Info("⏳ [IMAP] Attempting to connect to server...")

	c, err := client.DialTLS(s.Server, nil)
	if err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Connection failed")
		return err
	}
	defer c.Logout()

	logrus.WithField("server", s.Server).Info("✅ [IMAP] Connected successfully")

	if err := c.Login(s.Email, s.Password); err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Authentication failed")
		return err
	}
	logrus.Info("✅ [IMAP] Login successful")

	mbox, err := c.Select("INBOX", false)
	if err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Failed to select INBOX")
		return err
	}

	logrus.WithField("total_messages", mbox.Messages).Info("📥 [IMAP] INBOX status")

	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}

	ids, err := c.Search(criteria)
	if err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Failed to search for UNSEEN emails")
		return err
	}

	if len(ids) == 0 {
		logrus.Info("📭 [IMAP] No new emails to process")
		return nil
	}

	logrus.WithField("unseen_count", len(ids)).Info("📫 [IMAP] Found new emails, starting extraction...")

	seqset := new(imap.SeqSet)
	seqset.AddNum(ids...)

	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchEnvelope, section.FetchItem()}

	messages := make(chan *imap.Message, len(ids))
	done := make(chan error, 1)

	go func() {
		done <- c.Fetch(seqset, items, messages)
	}()

	for msg := range messages {
		if len(msg.Envelope.From) == 0 {
			continue
		}

		fromEmail := msg.Envelope.From[0].Address()
		messageID := msg.Envelope.MessageId
		date := msg.Envelope.Date

		logEntry := logrus.WithFields(logrus.Fields{
			"seq_num": msg.SeqNum,
			"msg_id":  messageID,
			"subject": msg.Envelope.Subject,
			"from":    fromEmail,
		})

		logEntry.Info("📨 [IMAP] Successfully fetched email envelope and body")

		var rawBody string
		r := msg.GetBody(section)
		if r != nil {
			bodyBytes, err := io.ReadAll(r)
			if err != nil {
				logEntry.WithError(err).Warn("⚠️ [IMAP] Failed to read email body bytes")
			} else {
				rawBody = string(bodyBytes)
			}
		}

		err := s.TxService.ProcessTransactionEmail(context.Background(), fromEmail, messageID, rawBody, date)
		if err != nil {
			logEntry.WithError(err).Error("❌ [IMAP] Failed to process transaction in service")
			continue
		}

		markSet := new(imap.SeqSet)
		markSet.AddNum(msg.SeqNum)
		flagOp := imap.FormatFlagsOp(imap.AddFlags, true)
		flags := []interface{}{imap.SeenFlag}

		if err := c.Store(markSet, flagOp, flags, nil); err != nil {
			logEntry.WithError(err).Warn("⚠️ [IMAP] Failed to mark email as SEEN")
		} else {
			logEntry.Info("✅ [IMAP] Email processed and marked as SEEN")
		}
	}

	if err := <-done; err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Error during envelope fetch")
		return err
	}

	return nil
}