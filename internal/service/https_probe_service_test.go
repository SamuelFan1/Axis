package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
)

type sequenceProbeDoer struct {
	mu         sync.Mutex
	results    []error
	requests   []*http.Request
	statusCode int
}

func (d *sequenceProbeDoer) Do(req *http.Request) (*http.Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.requests = append(d.requests, req)
	if len(d.results) > 0 {
		err := d.results[0]
		d.results = d.results[1:]
		if err != nil {
			return nil, err
		}
	}
	status := d.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("ok"))}, nil
}

func probeTestConfig() config.HTTPSProbeConfig {
	return config.HTTPSProbeConfig{
		Enabled:                    true,
		IntervalSec:                5,
		TimeoutMS:                  200,
		FailureThreshold:           3,
		RecoveryThreshold:          2,
		WorkerReconcileIntervalSec: 60,
	}
}

func TestHTTPSProbeServicePersistsThresholdsAcrossRestartAndRecovers(t *testing.T) {
	repo := &stubNodeRepository{
		nodes: map[string]node.Node{
			"node-1": {UUID: "node-1", Hostname: "SGP-NODE-1", DNSName: "dl-001.example.com", Region: "asia"},
		},
	}
	workerClient := &stubWorkerAdminClient{enabled: true}
	failing := &sequenceProbeDoer{results: []error{errors.New("timeout"), errors.New("timeout")}}
	first := NewHTTPSProbeService(repo, workerClient, probeTestConfig(), "asia")
	first.httpClient = failing
	if err := first.RunCycle(context.Background()); err != nil {
		t.Fatalf("first failure cycle: %v", err)
	}
	if err := first.RunCycle(context.Background()); err != nil {
		t.Fatalf("second failure cycle: %v", err)
	}
	if repo.probeStates["node-1"].Isolated {
		t.Fatal("node isolated before third failure")
	}

	second := NewHTTPSProbeService(repo, workerClient, probeTestConfig(), "asia")
	second.httpClient = &sequenceProbeDoer{results: []error{errors.New("timeout")}}
	if err := second.RunCycle(context.Background()); err != nil {
		t.Fatalf("third failure after restart: %v", err)
	}
	if !repo.probeStates["node-1"].Isolated {
		t.Fatal("expected persisted failures to isolate after restart")
	}
	if got := workerClient.probeRegions["asia"]; len(got) != 1 || got[0] != "api-origin-sgp-node-1" {
		t.Fatalf("unexpected isolated Worker set: %v", got)
	}

	second.httpClient = &sequenceProbeDoer{statusCode: http.StatusOK}
	if err := second.RunCycle(context.Background()); err != nil {
		t.Fatalf("first recovery cycle: %v", err)
	}
	if !repo.probeStates["node-1"].Isolated {
		t.Fatal("node recovered before second success")
	}
	if err := second.RunCycle(context.Background()); err != nil {
		t.Fatalf("second recovery cycle: %v", err)
	}
	if repo.probeStates["node-1"].Isolated {
		t.Fatal("node did not recover after two successes")
	}
	if got := workerClient.probeRegions["asia"]; len(got) != 0 {
		t.Fatalf("recovered node remained in Worker set: %v", got)
	}
}

func TestHTTPSProbeServiceChecksOnlyLocalRegionWithHTTPS443AndDeadline(t *testing.T) {
	repo := &stubNodeRepository{nodes: map[string]node.Node{
		"asia-node":   {UUID: "asia-node", Hostname: "SGP-NODE", DNSName: "dl-184.nuxdisk.com", Region: "asia"},
		"europe-node": {UUID: "europe-node", Hostname: "DE-NODE", DNSName: "dl-177.nuxdisk.com", Region: "europe"},
	}}
	doer := &sequenceProbeDoer{statusCode: http.StatusOK}
	service := NewHTTPSProbeService(repo, &stubWorkerAdminClient{enabled: true}, probeTestConfig(), "asia")
	service.httpClient = doer
	started := time.Now()
	if err := service.RunCycle(context.Background()); err != nil {
		t.Fatalf("RunCycle returned error: %v", err)
	}
	if len(doer.requests) != 1 {
		t.Fatalf("expected one local request, got %d", len(doer.requests))
	}
	req := doer.requests[0]
	if req.URL.String() != "https://dl-184.nuxdisk.com:443/health" {
		t.Fatalf("unexpected probe URL: %s", req.URL)
	}
	if req.Host != "dl-184.nuxdisk.com" {
		t.Fatalf("unexpected probe Host header: %s", req.Host)
	}
	deadline, ok := req.Context().Deadline()
	if !ok || deadline.Sub(started) > 250*time.Millisecond {
		t.Fatalf("expected a 200ms request deadline, got %v", deadline.Sub(started))
	}
}
