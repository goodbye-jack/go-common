package sms

import "context"

type SendRequest struct {
	Phone   string
	Content string
}

type SendResult struct {
	Provider             string
	ProviderRequestID    string
	ProviderResponseJSON string
}

type Provider interface {
	Name() string
	Send(ctx context.Context, req SendRequest) (*SendResult, error)
}

type ProviderError struct {
	Provider string
	Code     string
	Message  string
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}
