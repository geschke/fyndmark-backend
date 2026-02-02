package hcaptcha

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
		return nil, fmt.Errorf("hcaptcha secret key is not configured")
	}
	return &Provider{SecretKey: secretKey}, nil
}

// Validate checks a hCaptcha token against hCaptcha's API.
func (p *Provider) Validate(token, remoteIP string) (bool, []string, error) {
	return verify(token, remoteIP, p.SecretKey)
}

func verify(token, remoteIP, secret string) (bool, []string, error) {
	if secret == "" {
		return false, nil, fmt.Errorf("hcaptcha secret key is not configured")
	}

	data := url.Values{}
	data.Set("secret", secret)
	data.Set("response", token)
	if remoteIP != "" {
		data.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		"https://hcaptcha.com/siteverify",
		bytes.NewBufferString(data.Encode()),
	)
	if err != nil {
		return false, nil, fmt.Errorf("failed to create hCaptcha request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return false, nil, fmt.Errorf("hcaptcha verify request failed: %w", err)
	}
	defer resp.Body.Close()

	var vr VerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return false, nil, fmt.Errorf("failed to decode hCaptcha response: %w", err)
	}

	return vr.Success, vr.ErrorCodes, nil
}
