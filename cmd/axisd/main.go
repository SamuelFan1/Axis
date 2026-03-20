package main

import (
	"context"
	"log"

	"github.com/SamuelFan1/Axis/internal/bootstrap"
	"github.com/SamuelFan1/Axis/internal/config"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
	platformrouting "github.com/SamuelFan1/Axis/internal/platform/routingpublish"
	"github.com/SamuelFan1/Axis/internal/platform/workeradmin"
	"github.com/SamuelFan1/Axis/internal/repository/mysql"
	"github.com/SamuelFan1/Axis/internal/service"
	httptransport "github.com/SamuelFan1/Axis/internal/transport/http"
	"github.com/SamuelFan1/Axis/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	dbs, err := bootstrap.OpenDBSet(cfg)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer dbs.Close()

	nodeRepo := mysql.NewNodeRepository(dbs.Runtime)
	regionRepo := mysql.NewRegionRepository(dbs.Core)
	zoneRepo := mysql.NewZoneRepository(dbs.Core)

	dnsProvider := platformdns.NewNoopProvider()
	var dnsBindingRepo *mysql.DNSBindingRepository
	if cfg.DNS.Enabled && cfg.DNS.Provider == "cloudflare" {
		dnsProvider = platformdns.NewCloudflareProvider(cfg.DNS)
		dnsBindingRepo = mysql.NewDNSBindingRepository(dbs.Core)
	}
	workerAdminClient := workeradmin.NewClient(cfg.WorkerAdmin)
	regionService := service.NewRegionService(regionRepo, nodeRepo, zoneRepo, cfg.Region)
	zoneService := service.NewZoneService(zoneRepo, regionRepo, nodeRepo, nodeRepo, cfg.Region)
	nodeService := service.NewNodeService(nodeRepo, nodeRepo, nodeRepo, regionRepo, zoneRepo, dnsProvider, dnsBindingRepo, cfg.DNS, cfg.Region, workerAdminClient)
	ctx := context.Background()
	if cfg.App.AutoSchemaUpgrade {
		if err := regionRepo.EnsureSchema(ctx); err != nil {
			log.Fatalf("ensure region schema: %v", err)
		}
		if err := zoneRepo.EnsureSchema(ctx); err != nil {
			log.Fatalf("ensure zone schema: %v", err)
		}
		if err := nodeService.EnsureSchema(ctx); err != nil {
			log.Fatalf("ensure schema: %v", err)
		}
		if err := regionService.EnsureConfigured(ctx); err != nil {
			log.Fatalf("ensure configured regions: %v", err)
		}
		if err := zoneService.EnsureConfigured(ctx); err != nil {
			log.Fatalf("ensure configured zones: %v", err)
		}
		if err := zoneRepo.EnsureConstraints(ctx); err != nil {
			log.Fatalf("ensure zone relational constraints: %v", err)
		}
	} else {
		log.Print("startup schema upgrade disabled; skipping region/zone/node EnsureSchema")
	}
	if cfg.App.AutoSchemaUpgrade {
		if err := regionRepo.MigrateNodesRegionUUID(ctx); err != nil {
			log.Fatalf("migrate region_uuid: %v", err)
		}
		if err := zoneRepo.MigrateNodesZoneUUID(ctx); err != nil {
			log.Fatalf("migrate zone_uuid: %v", err)
		}
		if dnsBindingRepo != nil {
			seedResult, err := dnsBindingRepo.SeedFromManagedNodes(ctx, cfg.DNS.Zone, cfg.DNS.RecordPrefix)
			if err != nil {
				log.Fatalf("seed dns bindings from managed_nodes: %v", err)
			}
			floor := maxInt(seedResult.ManagedNodesMaxSequence, seedResult.DNSBindingsMaxSequence)
			if inspector, ok := dnsProvider.(platformdns.SequenceInspector); ok {
				cloudflareMaxSequence, err := inspector.MaxSequence(ctx, cfg.DNS.RecordPrefix)
				if err != nil {
					log.Fatalf("inspect cloudflare dns max sequence: %v", err)
				}
				floor = maxInt(floor, cloudflareMaxSequence)
			}
			if err := dnsBindingRepo.EnsureCounterFloor(ctx, cfg.DNS.Zone, cfg.DNS.RecordPrefix, floor); err != nil {
				log.Fatalf("ensure dns binding counter floor: %v", err)
			}
		}
	} else {
		log.Print("startup data reconciliation disabled; skipping managed_nodes backfill and dns seed")
	}

	var routingHandler *httptransport.RoutingHandler
	if cfg.Routing.Enabled {
		observationRepo := mysql.NewObservationRepository(dbs.Runtime)
		snapshotRepo := mysql.NewRoutingSnapshotRepository(dbs.Derived)

		if cfg.App.AutoSchemaUpgrade {
			if cfg.Routing.ObservationEnabled || cfg.Routing.SnapshotEnabled {
				if err := observationRepo.EnsureSchema(context.Background()); err != nil {
					log.Fatalf("ensure routing observation schema: %v", err)
				}
			}
			if cfg.Routing.SnapshotEnabled {
				if err := snapshotRepo.EnsureSchema(context.Background()); err != nil {
					log.Fatalf("ensure routing snapshot schema: %v", err)
				}
			}
		} else {
			log.Print("startup schema upgrade disabled; skipping routing EnsureSchema")
		}

		var observationService *service.RoutingObservationService
		if cfg.Routing.ObservationEnabled {
			observationService = service.NewRoutingObservationService(observationRepo)
		}

		var snapshotService *service.RoutingSnapshotService
		if cfg.Routing.SnapshotEnabled {
			snapshotService = service.NewRoutingSnapshotService(observationRepo, snapshotRepo, nodeRepo, regionRepo, zoneRepo, cfg.Routing)
		}

		publisher := platformrouting.NewNoopPublisher()
		if cfg.Routing.PublisherEnabled {
			publisher = platformrouting.NewCloudflareKVPublisher(cfg.Routing)
		}
		publishService := service.NewRoutingPublishService(publisher)

		if observationService != nil || snapshotService != nil {
			routingHandler = httptransport.NewRoutingHandler(observationService, snapshotService, publishService)
		}
		if snapshotService != nil && publishService.Enabled() && cfg.Region.LocalRegion == cfg.App.AuthoritativeRegion {
			routingPublisher := worker.NewRoutingSnapshotPublisher(
				snapshotService,
				publishService,
				cfg.Routing.PublishIntervalSec,
			)
			go routingPublisher.Run()
		}
	}

	nodeMonitor := worker.NewNodeMonitor(
		nodeService,
		cfg.App.NodeTimeoutSec,
		cfg.App.NodeMonitorIntervalSec,
	)
	go nodeMonitor.Run()

	server := httptransport.NewServer(cfg.App.HTTPAddress, cfg.Auth, nodeService, regionService, zoneService, routingHandler)
	if err := server.Run(); err != nil {
		log.Fatalf("run http server: %v", err)
	}
}

func maxInt(items ...int) int {
	best := 0
	for _, item := range items {
		if item > best {
			best = item
		}
	}
	return best
}
