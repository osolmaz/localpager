package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var sendDiscordMessageFunc = sendDiscordMessage

func sendDiscordMessage(ctx context.Context, token, channelID, content string) (string, error) {
	payload, err := json.Marshal(discordMessagePayload{
		Content: content,
		AllowedMentions: discordAllowedMentions{
			Parse: []string{},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://discord.com/api/v10/channels/"+channelID+"/messages", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var decoded struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&decoded)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &DiscordSendError{StatusCode: resp.StatusCode, Message: decoded.Message}
	}
	if decoded.ID == "" {
		return "", fmt.Errorf("discord send succeeded without message id")
	}
	return decoded.ID, nil
}

type DiscordSendError struct {
	StatusCode int
	Message    string
}

func (err *DiscordSendError) Error() string {
	if err.Message != "" {
		return fmt.Sprintf("discord send failed status=%d message=%s", err.StatusCode, err.Message)
	}
	return fmt.Sprintf("discord send failed status=%d", err.StatusCode)
}

func isPermanentDiscordSendError(err error) bool {
	var discordErr *DiscordSendError
	if !errors.As(err, &discordErr) {
		return false
	}
	return discordErr.StatusCode >= http.StatusBadRequest &&
		discordErr.StatusCode < http.StatusInternalServerError &&
		discordErr.StatusCode != http.StatusTooManyRequests
}

type discordMessagePayload struct {
	Content         string                 `json:"content"`
	AllowedMentions discordAllowedMentions `json:"allowed_mentions"`
}

type discordAllowedMentions struct {
	Parse []string `json:"parse"`
}
