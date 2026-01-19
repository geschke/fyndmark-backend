package turnstile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type VerifyResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

// Verify checks a Turnstile token against Cloudflare's API.
// If enabled is false, verification is skipped and treated as success.
func Validate(token, remoteIP, secret string, enabled bool) (bool, []string, error) {
	if !enabled {
		// Turnstile disabled for this form → always succeed.
		return true, nil, nil
	}

	if secret == "" {
		return false, nil, fmt.Errorf("turnstile secret key is not configured")
	}

	data := url.Values{}
	data.Set("secret", secret)
	data.Set("response", token)
	if remoteIP != "" {
		data.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		"https://challenges.cloudflare.com/turnstile/v0/siteverify",
		bytes.NewBufferString(data.Encode()),
	)
	if err != nil {
		return false, nil, fmt.Errorf("failed to create Turnstile request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return false, nil, fmt.Errorf("turnstile verify request failed: %w", err)
	}
	defer resp.Body.Close()

	var vr VerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return false, nil, fmt.Errorf("failed to decode Turnstile response: %w", err)
	}

	return vr.Success, vr.ErrorCodes, nil
}
