package workers

import (
	"io"
	"walletx-be/pkg/services"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/sirupsen/logrus"
)

// IMAPWorker handles processing of incoming emails
type IMAPWorker struct {
	Server    string
	Email     string
	Password  string
	TxService services.TransactionService
}

// NewIMAPWorker creates a new IMAPWorker instance
func NewIMAPWorker(email, password string, txService services.TransactionService) *IMAPWorker {
	return &IMAPWorker{
		Server:    "imap.gmail.com:993",
		Email:     email,
		Password:  password,
		TxService: txService,
	}
}

// ProcessUnseenEmails connects to the IMAP server, fetches unread emails, and triggers processing
func (p *IMAPWorker) ProcessUnseenEmails() error {
	logrus.Info("⏳ [IMAP] Attempting to connect to server...")

	// Connect to IMAP server
	c, err := client.DialTLS(p.Server, nil)
	if err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Connection failed")
		return err
	}
	defer c.Logout()

	logrus.WithField("server", p.Server).Info("✅ [IMAP] Connected successfully")

	// Authenticate
	if err := c.Login(p.Email, p.Password); err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Authentication failed")
		return err
	}
	logrus.Info("✅ [IMAP] Login successful")

	// Select INBOX
	mbox, err := c.Select("INBOX", false)
	if err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Failed to select INBOX")
		return err
	}

	logrus.WithField("total_messages", mbox.Messages).Info("📥 [IMAP] INBOX status")

	// Search for UNSEEN emails
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

	// Prepare to fetch data
	seqset := new(imap.SeqSet)
	seqset.AddNum(ids...)

	// Define the body section to fetch the actual email content
	section := &imap.BodySectionName{}

	// Add the body section to the items we want to fetch
	items := []imap.FetchItem{imap.FetchEnvelope, section.FetchItem()}

	// Channel to receive incoming messages
	messages := make(chan *imap.Message, len(ids))
	done := make(chan error, 1)

	// Fetch envelopes and bodies asynchronously
	go func() {
		done <- c.Fetch(seqset, items, messages)
	}()

	// Iterate through fetched messages
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

		// Extract raw body text
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

		// Pass the real rawBody to the Transaction Service
		err := p.TxService.ProcessTransactionEmail(fromEmail, messageID, rawBody, date)
		if err != nil {
			logEntry.WithError(err).Error("❌ [IMAP] Failed to process transaction in service")
			continue 
		}

		// Mark email as SEEN only if processing was successful
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

	// Wait for fetch completion
	if err := <-done; err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Error during envelope fetch")
		return err
	}

	return nil
}