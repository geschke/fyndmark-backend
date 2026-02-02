package turnstile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Provider struct {
	SecretKey string
}

type VerifyResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

func New(secretKey string) (*Provider, error) {
	if secretKey == "" {
		return nil, fmt.Errorf("turnstile secret key is not configured")
	}
	return &Provider{SecretKey: secretKey}, nil
}

// Validate checks a Turnstile token against Cloudflare's API.
func (p *Provider) Validate(token, remoteIP string) (bool, []string, error) {
	return verify(token, remoteIP, p.SecretKey)
}

// Validate is a legacy helper that supports enabled/disabled toggles.
func Validate(token, remoteIP, secret string, enabled bool) (bool, []string, error) {
	if !enabled {
		// Turnstile disabled for this form → always succeed.
		return true, nil, nil
	}
	return verify(token, remoteIP, secret)
}

func verify(token, remoteIP, secret string) (bool, []string, error) {
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
