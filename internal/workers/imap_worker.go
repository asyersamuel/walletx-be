package workers

import (
	"walletx-be/internal/services"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/sirupsen/logrus"
)

// IMAPWorker handles processing of incoming emails
type IMAPWorker struct {
	Server    string
	Email     string
	Password  string
	TxService *services.TransactionService
}

// NewIMAPWorker creates a new IMAPWorker instance
func NewIMAPWorker(email, password string, txService *services.TransactionService) *IMAPWorker {
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

	// Channel to receive incoming messages
	messages := make(chan *imap.Message, len(ids))
	done := make(chan error, 1)

	// Fetch envelopes asynchronously
	go func() {
		done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
	}()

	// Iterate through fetched messages
	for msg := range messages {
		logEntry := logrus.WithFields(logrus.Fields{
			"seq_num": msg.SeqNum,
			"msg_id":  msg.Envelope.MessageId,
			"subject": msg.Envelope.Subject,
		})

		if len(msg.Envelope.From) > 0 {
			fromEmail := msg.Envelope.From[0].Address()
			logEntry = logEntry.WithField("from", fromEmail)
		}

		logEntry.Info("📨 [IMAP] Successfully fetched email envelope")

		// TODO: Implement database check for registered sender email here
	}

	// Wait for fetch completion
	if err := <-done; err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Error during envelope fetch")
		return err
	}

	return nil
}