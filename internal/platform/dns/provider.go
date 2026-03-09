package dns

import "context"

type Record struct {
	Name    string
	Type    string
	Content string
	TTL     int
	Proxied bool
}

type Provider interface {
	EnsureRecord(ctx context.Context, record Record) error
	Enabled() bool
}
