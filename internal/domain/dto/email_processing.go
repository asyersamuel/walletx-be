package dto

import "time"

type EmailProcessingPayload struct {
	MessageID string    `json:"message_id"`
	Sender    string    `json:"sender"`
	Date      time.Time `json:"date"`
	Body      string    `json:"body"`
}
