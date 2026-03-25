package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"hashtasker-go/internal/models"
)

var webhookClient = &http.Client{Timeout: 10 * time.Second}

// SendJobWebhook sends a webhook notification for a job status change.
// The payload format is {"msg": "..."} as documented.
func SendJobWebhook(job *models.Job, message string) {
	if job.WebhookURL == "" {
		return
	}

	payload := map[string]string{"msg": message}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Webhook: failed to marshal payload for job %s: %v", job.UID, err)
		return
	}

	go func() {
		resp, err := webhookClient.Post(job.WebhookURL, "application/json", bytes.NewBuffer(data))
		if err != nil {
			log.Printf("Webhook: failed to send for job %s to %s: %v", job.UID, job.WebhookURL, err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("Webhook: sent for job %s (%s)", job.UID, message)
		} else {
			log.Printf("Webhook: job %s returned status %d from %s", job.UID, resp.StatusCode, job.WebhookURL)
		}
	}()
}
