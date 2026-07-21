package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/domain/routing"
	"github.com/SamuelFan1/Axis/internal/platform/workeradmin"
	"github.com/SamuelFan1/Axis/internal/repository"
)

type httpsProbeDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type HTTPSProbeService struct {
	repo              repository.NodeAvailabilityRepository
	workerAdmin       workeradmin.Client
	cfg               config.HTTPSProbeConfig
	localRegion       string
	owner             string
	httpClient        httpsProbeDoer
	now               func() time.Time
	lastPublishedHash string
	lastPublishedAt   time.Time
}

func NewHTTPSProbeService(repo repository.NodeAvailabilityRepository, workerAdmin workeradmin.Client, cfg config.HTTPSProbeConfig, localRegion string) *HTTPSProbeService {
	hostname, _ := os.Hostname()
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &HTTPSProbeService{
		repo:        repo,
		workerAdmin: workerAdmin,
		cfg:         cfg,
		localRegion: strings.TrimSpace(strings.ToLower(localRegion)),
		owner:       fmt.Sprintf("%s:%d", hostname, os.Getpid()),
		httpClient:  client,
		now:         func() time.Time { return time.Now().UTC() },
	}
}

func (s *HTTPSProbeService) RunCycle(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("HTTPS probe repository is not configured")
	}
	items, err := s.repo.ListIdentitiesByRegion(ctx, s.localRegion)
	if err != nil {
		return err
	}
	identityByUUID := make(map[string]node.NodeIdentity, len(items))
	for _, item := range items {
		identityByUUID[item.UUID] = item
	}

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var cycleErrs []error
	for _, item := range items {
		item := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.probeNode(ctx, item); err != nil {
				errMu.Lock()
				cycleErrs = append(cycleErrs, fmt.Errorf("probe node %s: %w", item.UUID, err))
				errMu.Unlock()
			}
		}()
	}
	wg.Wait()

	if _, err := s.repo.CleanupOrphanedHTTPSProbeRows(ctx, s.localRegion); err != nil {
		cycleErrs = append(cycleErrs, err)
	}
	if err := s.publishRegionalBlacklist(ctx, identityByUUID); err != nil {
		cycleErrs = append(cycleErrs, err)
	}
	return errors.Join(cycleErrs...)
}

func (s *HTTPSProbeService) probeNode(ctx context.Context, item node.NodeIdentity) error {
	now := s.now()
	claimed, err := s.repo.TryClaimHTTPSProbe(ctx, s.localRegion, item.UUID, s.owner, now, time.Second)
	if err != nil || !claimed {
		return err
	}
	result := s.checkPublicHTTPS(ctx, item.DNSName, now)
	_, _, err = s.repo.RecordHTTPSProbeResult(
		ctx,
		s.localRegion,
		item.UUID,
		s.owner,
		result,
		s.cfg.FailureThreshold,
		s.cfg.RecoveryThreshold,
		time.Duration(s.cfg.IntervalSec)*time.Second,
	)
	return err
}

func (s *HTTPSProbeService) checkPublicHTTPS(parent context.Context, dnsName string, checkedAt time.Time) node.HTTPSProbeResult {
	host := strings.TrimSpace(strings.TrimSuffix(dnsName, "."))
	result := node.HTTPSProbeResult{CheckedAt: checkedAt}
	if host == "" {
		result.Error = "dns_name is empty"
		return result
	}
	if strings.Contains(host, "://") {
		result.Error = "dns_name contains a scheme"
		return result
	}
	target := url.URL{Scheme: "https", Host: net.JoinHostPort(host, "443"), Path: "/health"}
	timeout := time.Duration(s.cfg.TimeoutMS) * time.Millisecond
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	// Caddy host matchers expect the DNS name without an explicit default port.
	// The URL still dials 443, while the HTTP Host header remains canonical.
	req.Host = host
	resp, err := s.httpClient.Do(req)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	result.HTTPStatus = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("HTTP status %d", resp.StatusCode)
		return result
	}
	result.Healthy = true
	return result
}

func (s *HTTPSProbeService) publishRegionalBlacklist(ctx context.Context, identityByUUID map[string]node.NodeIdentity) error {
	states, err := s.repo.ListIsolatedHTTPSProbeStates(ctx, s.localRegion)
	if err != nil {
		return err
	}
	labels := make([]string, 0, len(states))
	for _, state := range states {
		identity, ok := identityByUUID[state.NodeUUID]
		if !ok {
			continue
		}
		label := routing.OriginLabelForHostname(identity.Hostname)
		if label != "" {
			labels = append(labels, label)
		}
	}
	sort.Strings(labels)
	hash := strings.Join(labels, "\x00")
	now := s.now()
	reconcileInterval := time.Duration(s.cfg.WorkerReconcileIntervalSec) * time.Second
	if hash == s.lastPublishedHash && !s.lastPublishedAt.IsZero() && now.Sub(s.lastPublishedAt) < reconcileInterval {
		return nil
	}
	if err := s.workerAdmin.ReplaceHTTPSProbeBlacklist(ctx, s.localRegion, labels); err != nil {
		return fmt.Errorf("publish regional HTTPS probe blacklist: %w", err)
	}
	s.lastPublishedHash = hash
	s.lastPublishedAt = now
	return nil
}
