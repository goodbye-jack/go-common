package sms

import "context"

type MockProvider struct{}

func (p MockProvider) Name() string {
	return ProviderMock
}

func (p MockProvider) Send(context.Context, SendRequest) (*SendResult, error) {
	return &SendResult{
		Provider: ProviderMock,
	}, nil
}
