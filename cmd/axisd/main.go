package main

import (
	"context"
	"log"

	"github.com/SamuelFan1/Axis/internal/bootstrap"
	"github.com/SamuelFan1/Axis/internal/config"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
	platformrouting "github.com/SamuelFan1/Axis/internal/platform/routingpublish"
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

	db, err := bootstrap.OpenDB(cfg.DB)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	nodeRepo := mysql.NewNodeRepository(db)
	regionRepo := mysql.NewRegionRepository(db)
	zoneRepo := mysql.NewZoneRepository(db)
	dnsProvider := platformdns.NewNoopProvider()
	if cfg.DNS.Enabled && cfg.DNS.Provider == "cloudflare" {
		dnsProvider = platformdns.NewCloudflareProvider(cfg.DNS)
	}
	regionService := service.NewRegionService(regionRepo, nodeRepo, cfg.Region)
	zoneService := service.NewZoneService(zoneRepo, nodeRepo, cfg.Region)
	nodeService := service.NewNodeService(nodeRepo, regionRepo, zoneRepo, dnsProvider, cfg.DNS, cfg.Region)
	if err := regionRepo.EnsureSchema(context.Background()); err != nil {
		log.Fatalf("ensure region schema: %v", err)
	}
	if err := zoneRepo.EnsureSchema(context.Background()); err != nil {
		log.Fatalf("ensure zone schema: %v", err)
	}
	if err := nodeService.EnsureSchema(context.Background()); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}
	if err := regionRepo.MigrateNodesRegionUUID(context.Background()); err != nil {
		log.Fatalf("migrate region_uuid: %v", err)
	}
	if err := zoneRepo.MigrateNodesZoneUUID(context.Background()); err != nil {
		log.Fatalf("migrate zone_uuid: %v", err)
	}

	var routingHandler *httptransport.RoutingHandler
	if cfg.Routing.Enabled {
		observationRepo := mysql.NewObservationRepository(db)
		snapshotRepo := mysql.NewRoutingSnapshotRepository(db)

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

		var observationService *service.RoutingObservationService
		if cfg.Routing.ObservationEnabled {
			observationService = service.NewRoutingObservationService(observationRepo)
		}

		var snapshotService *service.RoutingSnapshotService
		if cfg.Routing.SnapshotEnabled {
			snapshotService = service.NewRoutingSnapshotService(observationRepo, snapshotRepo, nodeRepo, cfg.Routing)
		}

		publisher := platformrouting.NewNoopPublisher()
		if cfg.Routing.PublisherEnabled {
			publisher = platformrouting.NewCloudflareKVPublisher(cfg.Routing)
		}
		publishService := service.NewRoutingPublishService(publisher)

		if observationService != nil || snapshotService != nil {
			routingHandler = httptransport.NewRoutingHandler(observationService, snapshotService, publishService)
		}
		if snapshotService != nil && publishService.Enabled() {
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
