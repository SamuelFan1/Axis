package repository

import (
	"context"

	"github.com/SamuelFan1/Axis/internal/domain/dnsbinding"
)

type DNSBindingSeedResult struct {
	ManagedNodesMaxSequence int
	DNSBindingsMaxSequence  int
	SeededCount             int
}

type DNSBindingRepository interface {
	EnsureSchema(ctx context.Context) error
	GetByNodeUUID(ctx context.Context, nodeUUID string) (*dnsbinding.Binding, error)
	GetByDNSLabel(ctx context.Context, label string) (*dnsbinding.Binding, error)
	AllocateForNode(ctx context.Context, nodeUUID string, zone string, prefix string) (*dnsbinding.Binding, error)
	UpdateLastPublicIP(ctx context.Context, nodeUUID string, publicIP string) error
	SeedFromManagedNodes(ctx context.Context, zone string, prefix string) (DNSBindingSeedResult, error)
	EnsureCounterFloor(ctx context.Context, zone string, prefix string, floor int) error
}
