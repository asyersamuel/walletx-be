package services

import (
	"context"
	"io"

	"walletx-be/internal/domain/dto"
	"walletx-be/internal/ports"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

type IMAPService struct {
	Server        string
	Email         string
	Password      string
	queueService  ports.MessageQueueService
	logger        ports.Logger
}

func NewIMAPService(email, password string, queueService ports.MessageQueueService, logger ports.Logger) *IMAPService {
	return &IMAPService{
		Server:       "imap.gmail.com:993",
		Email:        email,
		Password:     password,
		queueService: queueService,
		logger:       logger,
	}
}

func (s *IMAPService) ProcessUnseenEmails(ctx context.Context) error {
	s.logger.Info("Attempting to connect to IMAP server...")

	c, err := client.DialTLS(s.Server, nil)
	if err != nil {
		s.logger.WithError(err).Error("IMAP connection failed")
		return err
	}
	defer c.Logout()

	s.logger.WithField("server", s.Server).Info("Connected to IMAP server successfully")

	if err := c.Login(s.Email, s.Password); err != nil {
		s.logger.WithError(err).Error("IMAP authentication failed")
		return err
	}
	s.logger.Info("IMAP login successful")

	mbox, err := c.Select("INBOX", false)
	if err != nil {
		s.logger.WithError(err).Error("Failed to select INBOX")
		return err
	}

	s.logger.WithField("total_messages", mbox.Messages).Info("INBOX status received")

	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}

	ids, err := c.Search(criteria)
	if err != nil {
		s.logger.WithError(err).Error("Failed to search for UNSEEN emails")
		return err
	}

	if len(ids) == 0 {
		s.logger.Info("No new emails to process")
		return nil
	}

	s.logger.WithField("unseen_count", len(ids)).Info("Found new emails, starting extraction...")

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
		select {
		case <-ctx.Done():
			s.logger.Warn("Context cancelled, stopping IMAP sync")
			return ctx.Err()
		default:
		}

		if len(msg.Envelope.From) == 0 {
			continue
		}

		senderEmail := msg.Envelope.From[0].Address()
		messageID := msg.Envelope.MessageId
		date := msg.Envelope.Date

		var recipientEmail string
		if len(msg.Envelope.To) > 0 {
			recipientEmail = msg.Envelope.To[0].Address()
		}

		logger := s.logger.WithField("seq_num", msg.SeqNum).
			WithField("msg_id", messageID).
			WithField("subject", msg.Envelope.Subject).
			WithField("from", senderEmail).
			WithField("to", recipientEmail)

		logger.Info("Successfully fetched email envelope and body")

		if recipientEmail == "" {
			logger.Warn("No recipient found in email envelope, skipping")
			continue
		}

		var rawBody string
		r := msg.GetBody(section)
		if r != nil {
			bodyBytes, err := io.ReadAll(r)
			if err != nil {
				logger.WithError(err).Warn("Failed to read email body bytes")
			} else {
				rawBody = string(bodyBytes)
			}
		}

		payload := dto.EmailProcessingPayload{
			MessageID: messageID,
			Sender:    senderEmail,
			Date:      date,
			Body:      rawBody,
		}

		err = s.queueService.Publish(ctx, payload)
		if err != nil {
			logger.WithError(err).Error("Failed to publish email to queue, skipping SEEN flag")
			continue
		}

		markSet := new(imap.SeqSet)
		markSet.AddNum(msg.SeqNum)
		flagOp := imap.FormatFlagsOp(imap.AddFlags, true)
		flags := []interface{}{imap.SeenFlag}

		if err := c.Store(markSet, flagOp, flags, nil); err != nil {
			logger.WithError(err).Warn("Failed to mark email as SEEN")
		} else {
			logger.Info("Email published and marked as SEEN")
		}
	}

	if err := <-done; err != nil {
		s.logger.WithError(err).Error("Error during envelope fetch")
		return err
	}

	return nil
}