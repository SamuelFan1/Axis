package dns

import "context"

type Record struct {
	Name    string
	Type    string
	Content string
	TTL     int
	Proxied bool
}

type ManagedRecord struct {
	ID      string
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

type RecordManager interface {
	ListRecords(ctx context.Context, recordType string) ([]ManagedRecord, error)
	CreateRecord(ctx context.Context, record Record) error
	UpdateRecord(ctx context.Context, id string, record Record) error
	DeleteRecord(ctx context.Context, id string) error
}

type SequenceInspector interface {
	MaxSequence(ctx context.Context, prefix string) (int, error)
}
