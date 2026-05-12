package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"walletx-be/configs"
	"walletx-be/internal/domain/dto"
	"walletx-be/internal/ports"
)

type UpstashQStashService struct {
	httpClient  *http.Client
	token       string
	publishURL  string
	webhookURL  string
	logger      ports.Logger
}

func NewUpstashQStashService(cfg configs.QStashConfig, logger ports.Logger) *UpstashQStashService {
	return &UpstashQStashService{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token:      cfg.Token,
		publishURL: cfg.URL,
		webhookURL: cfg.WebhookBaseURL + "/api/v1/internal/webhooks/email-processor",
		logger:     logger,
	}
}

func (s *UpstashQStashService) Publish(ctx context.Context, payload dto.EmailProcessingPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		s.logger.WithError(err).Error("Failed to marshal email payload")
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	targetURL := s.publishURL + s.webhookURL

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		s.logger.WithError(err).Error("Failed to create QStash request")
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.WithError(err).Error("Failed to publish to QStash")
		return fmt.Errorf("failed to publish to QStash: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		s.logger.WithField("status_code", resp.StatusCode).Error("QStash returned error status")
		return fmt.Errorf("qstash returned status %d", resp.StatusCode)
	}

	s.logger.WithField("message_id", payload.MessageID).Info("Successfully published to QStash")
	return nil
}
