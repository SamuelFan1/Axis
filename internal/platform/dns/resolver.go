package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
)

type Resolver interface {
	LookupA(ctx context.Context, host string) ([]string, error)
}

type NetResolver struct {
	resolver *net.Resolver
}

func NewNetResolver() Resolver {
	return &NetResolver{
		resolver: net.DefaultResolver,
	}
}

func (r *NetResolver) LookupA(ctx context.Context, host string) ([]string, error) {
	host = strings.Trim(strings.TrimSpace(host), ".")
	if host == "" {
		return nil, fmt.Errorf("dns host is required")
	}

	addresses, err := r.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		if isNotFoundDNSError(err) {
			return nil, nil
		}
		return nil, err
	}

	seen := make(map[string]struct{})
	results := make([]string, 0, len(addresses))
	for _, address := range addresses {
		ipv4 := address.IP.To4()
		if ipv4 == nil {
			continue
		}
		value := ipv4.String()
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		results = append(results, value)
	}

	sort.Strings(results)
	return results, nil
}

func isNotFoundDNSError(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}
