package workers

import (
	"walletx-be/internal/services"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/sirupsen/logrus"
)

// IMAPWorker bertanggung jawab memproses email masuk
type IMAPWorker struct {
	Server   string
	Email    string
	Password string
	TxService *services.TransactionService
}

// NewIMAPWorker membuat instance IMAPWorker Baru
func NewIMAPWorker(email, password string, txService *services.TransactionService) *IMAPWorker {
	return &IMAPWorker{
		Server:   "imap.gmail.com:993",
		Email:    email,
		Password: password,
		TxService: txService,
	}
}

// ProcessUnseenEmails melakukan Koneksi, Membaca, dan Mengubah Status
func (p *IMAPWorker) ProcessUnseenEmails() error {
	logrus.Info("⏳ [IMAP] Mencoba terhubung ke server...")

	// 1. Konek ke Server
	c, err := client.DialTLS(p.Server, nil)
	if err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Gagal terhubung")
		return err
	}
	defer c.Logout()

	logrus.WithField("server", p.Server).Info("✅ [IMAP] Berhasil terhubung")

	// 2. Autentikasi
	if err := c.Login(p.Email, p.Password); err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Gagal login")
		return err
	}
	logrus.Info("✅ [IMAP] Berhasil Login!")

	// 3. Pilih Kotak Masuk
	mbox, err := c.Select("INBOX", false)
	if err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Gagal membuka INBOX")
		return err
	}

	logrus.WithField("total_messages", mbox.Messages).Info("📥 [IMAP] Status INBOX")

	// 4. Cari Email UNSEEN
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}

	ids, err := c.Search(criteria)
	if err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Gagal mencari email UNSEEN")
		return err
	}

	if len(ids) == 0 {
		logrus.Info("📭 [IMAP] Tidak ada email baru untuk diproses.")
		return nil
	}

	logrus.WithField("unseen_count", len(ids)).Info("📫 [IMAP] Ditemukan email baru. Memulai ekstraksi...")

	// 5. Persiapkan pengambilan data (Fetch)
	seqset := new(imap.SeqSet)
	seqset.AddNum(ids...)

	// Channel untuk menampung pesan yang datang dari server
	messages := make(chan *imap.Message, len(ids))
	done := make(chan error, 1)

	// Mulai mengambil Envelope secara asynchronous
	go func() {
		done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
	}()

	// 6. Iterasi setiap email yang masuk ke channel
	for msg := range messages {
		// Gunakan Logrus dengan Fields agar terstruktur
		logEntry := logrus.WithFields(logrus.Fields{
			"seq_num": msg.SeqNum,
			"msg_id":  msg.Envelope.MessageId,
			"subject": msg.Envelope.Subject,
		})

		if len(msg.Envelope.From) > 0 {
			fromEmail := msg.Envelope.From[0].Address()
			logEntry = logEntry.WithField("from", fromEmail)
		}

		logEntry.Info("📨 [IMAP] Berhasil mengambil envelope email")

		// LOGIKA BERIKUTNYA:
		// Di sini nanti kita akan cek ke Database: 
		// "Apakah email pengirim ini terdaftar?"
	}

	// Tunggu sampai semua fetch selesai
	if err := <-done; err != nil {
		logrus.WithError(err).Error("❌ [IMAP] Error saat fetch envelope")
		return err
	}

	return nil
}