package dns

import "context"

type NoopProvider struct{}

func NewNoopProvider() Provider {
	return NoopProvider{}
}

func (NoopProvider) EnsureRecord(context.Context, Record) error {
	return nil
}

func (NoopProvider) Enabled() bool {
	return false
}
