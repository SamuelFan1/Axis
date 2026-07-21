package main

import (
	"context"
	"database/sql"
	"log"

	"github.com/SamuelFan1/Axis/internal/bootstrap"
	"github.com/SamuelFan1/Axis/internal/config"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
	platformrouting "github.com/SamuelFan1/Axis/internal/platform/routingpublish"
	"github.com/SamuelFan1/Axis/internal/platform/workeradmin"
	"github.com/SamuelFan1/Axis/internal/repository"
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

	nodeRepo := mysql.NewNodeRepository(dbs.Core, dbs.Runtime)
	regionRepo := mysql.NewRegionRepository(dbs.Core)
	zoneRepo := mysql.NewZoneRepository(dbs.Core)
	var aggregatedRepo *mysql.AggregatedNodeRepository
	var aggregationHandler *httptransport.AggregationHandler
	ctx := context.Background()

	dnsProvider := platformdns.NewNoopProvider()
	var dnsBindingRepo *mysql.DNSBindingRepository
	if (cfg.DNS.Enabled || cfg.DNS.AuxiliaryEnabled || cfg.HQ.Enabled) && cfg.DNS.Provider == "cloudflare" {
		dnsProvider = platformdns.NewCloudflareProvider(cfg.DNS)
		if cfg.DNS.Enabled || cfg.HQ.Enabled {
			dnsBindingRepo = mysql.NewDNSBindingRepository(dbs.Core)
		}
	}
	workerAdminClient := workeradmin.NewClient(cfg.WorkerAdmin)
	regionService := service.NewRegionService(regionRepo, nodeRepo, zoneRepo, cfg.Region)
	zoneService := service.NewZoneService(zoneRepo, regionRepo, nodeRepo, nodeRepo, cfg.Region)
	var nodeService *service.NodeService
	if cfg.Aggregation.Enabled {
		snapshotInboxRepo := mysql.NewRegionalNodeStatusSnapshotRepository(dbs.Derived)
		aggregatedRepo = mysql.NewAggregatedNodeRepository(dbs.Derived)
		nodeService = service.NewNodeService(nodeRepo, nodeRepo, nodeRepo, aggregatedRepo, regionRepo, zoneRepo, dnsProvider, dnsBindingRepo, cfg.DNS, cfg.Region, workerAdminClient)
		if cfg.App.AutoSchemaUpgrade {
			if err := snapshotInboxRepo.EnsureSchema(ctx); err != nil {
				log.Fatalf("ensure aggregation snapshot schema: %v", err)
			}
			if err := aggregatedRepo.EnsureSchema(ctx); err != nil {
				log.Fatalf("ensure aggregation view schema: %v", err)
			}
		}
		if cfg.Aggregation.CentralReceiverEnabled {
			aggregationHandler = httptransport.NewAggregationHandler(snapshotInboxRepo)
		}
		if cfg.Aggregation.CentralAggregatorEnabled {
			var dnsBinder service.DNSBindingCallback
			if cfg.Region.LocalRegion == cfg.App.AuthoritativeRegion && cfg.DNS.Enabled {
				dnsBinder = nodeService.EnsureDNSBindingForNode
			}
			aggregatedNodeService := service.NewAggregatedNodeService(nodeRepo, snapshotInboxRepo, aggregatedRepo, cfg.Aggregation.StaleAfterSec, dnsBinder)
			go worker.NewGlobalNodeAggregator(aggregatedNodeService, cfg.Aggregation.PublishIntervalSec).Run()
		}
		if cfg.Aggregation.RegionalPublisherEnabled {
			regionalSnapshotService := service.NewRegionalStatusSnapshotService(nodeRepo, cfg.Region.LocalRegion)
			sink := httptransport.NewRegionalSnapshotPublisherClient(cfg.Aggregation)
			go worker.NewRegionalStatusSnapshotPublisher(regionalSnapshotService, sink, cfg.Aggregation.PublishIntervalSec).Run()
		}
		if !cfg.App.AutoSchemaUpgrade {
			if err := requireTables(ctx, dbs.Derived, "platform_derived", "regional_node_status_snapshots", "aggregated_node_status"); err != nil {
				log.Fatalf("aggregation enabled but derived schema is incomplete: %v", err)
			}
		}
	}
	if nodeService == nil {
		nodeService = service.NewNodeService(nodeRepo, nodeRepo, nodeRepo, aggregatedRepo, regionRepo, zoneRepo, dnsProvider, dnsBindingRepo, cfg.DNS, cfg.Region, workerAdminClient)
	}
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

	// RoutingSnapshotService filters UP nodes against platform_core.regions + zones. That seeding
	// used to run only when AXIS_AUTO_SCHEMA_UPGRADE=true, so with auto upgrade off the zones table
	// could stay empty while aggregated_node_status still showed nodes — producing empty KV manifests.
	if cfg.Routing.Enabled && cfg.Routing.SnapshotEnabled && !cfg.App.AutoSchemaUpgrade {
		log.Print("routing snapshot enabled without full auto schema upgrade; ensuring core regions/zones for routing candidate filters")
		if err := regionRepo.EnsureSchema(ctx); err != nil {
			log.Fatalf("routing: ensure region schema: %v", err)
		}
		if err := zoneRepo.EnsureSchema(ctx); err != nil {
			log.Fatalf("routing: ensure zone schema: %v", err)
		}
		if err := regionService.EnsureConfigured(ctx); err != nil {
			log.Fatalf("routing: ensure configured regions: %v", err)
		}
		if err := zoneService.EnsureConfigured(ctx); err != nil {
			log.Fatalf("routing: ensure configured zones: %v", err)
		}
		if err := zoneRepo.EnsureConstraints(ctx); err != nil {
			log.Fatalf("routing: ensure zone relational constraints: %v", err)
		}
	}

	var routingHandler *httptransport.RoutingHandler
	var observationRepo *mysql.ObservationRepository
	var publishService *service.RoutingPublishService
	if cfg.Routing.Enabled {
		observationRepo = mysql.NewObservationRepository(dbs.Runtime)
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
			var nodeViewRepo repository.NodeViewRepository = nodeRepo
			if aggregatedRepo != nil {
				nodeViewRepo = aggregatedRepo
			}
			snapshotService = service.NewRoutingSnapshotService(observationRepo, snapshotRepo, nodeViewRepo, regionRepo, zoneRepo, cfg.Routing)
		}

		publisher := platformrouting.NewNoopPublisher()
		if cfg.Routing.PublisherEnabled {
			publisher = platformrouting.NewCloudflareKVPublisher(cfg.Routing)
		}
		publishService = service.NewRoutingPublishService(publisher)

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

	if cfg.DNS.AuxiliaryEnabled {
		if cfg.Region.LocalRegion == cfg.App.AuthoritativeRegion {
			recordProvider := platformdns.NewCloudflareProviderForZone(cfg.DNS, cfg.DNS.AuxiliaryZone)
			recordManager, ok := recordProvider.(platformdns.RecordManager)
			if !ok {
				log.Fatalf("auxiliary dns enabled but dns provider does not support record management")
			}
			var nodeViewRepo repository.NodeViewRepository = nodeRepo
			if aggregatedRepo != nil {
				nodeViewRepo = aggregatedRepo
			}
			auxiliaryDNSService := service.NewAuxiliaryDNSService(nodeViewRepo, recordManager, cfg.DNS)
			go worker.NewAuxiliaryDNSSyncer(auxiliaryDNSService, cfg.DNS.AuxiliarySyncIntervalSec).Run()
		} else {
			log.Printf("auxiliary dns sync disabled on non-authoritative region: local=%s authoritative=%s", cfg.Region.LocalRegion, cfg.App.AuthoritativeRegion)
		}
	}

	if cfg.HQ.Enabled {
		if aggregatedRepo == nil {
			log.Fatalf("hq enabled but aggregated node repository is not configured")
		}
		if dnsBindingRepo == nil {
			log.Fatalf("hq enabled but dns binding repository is not configured")
		}
		if observationRepo == nil {
			log.Fatalf("hq enabled but routing observation repository is not configured")
		}
		if publishService == nil || !publishService.Enabled() {
			log.Fatalf("hq enabled but routing publisher is not configured")
		}
		hqDNSProvider := platformdns.NewCloudflareProviderForZone(cfg.DNS, cfg.HQ.DNSZone)
		hqDNSService := service.NewHQDNSService(aggregatedRepo, dnsBindingRepo, hqDNSProvider, cfg.HQ)
		go worker.NewHQDNSSyncer(hqDNSService, cfg.HQ.DNSSyncIntervalSec).Run()

		hqSnapshotService := service.NewHQRoutingSnapshotService(
			observationRepo,
			aggregatedRepo,
			regionRepo,
			zoneRepo,
			dnsBindingRepo,
			cfg.Routing,
			cfg.HQ,
		)
		go worker.NewHQRoutingSnapshotPublisher(
			hqSnapshotService,
			publishService,
			cfg.Routing.PublishIntervalSec,
		).Run()
	}

	nodeMonitor := worker.NewNodeMonitor(
		nodeService,
		cfg.App.NodeTimeoutSec,
		cfg.App.NodeMonitorIntervalSec,
	)
	go nodeMonitor.Run()

	if cfg.HTTPSProbe.Enabled {
		if cfg.App.AutoSchemaUpgrade {
			if err := nodeRepo.EnsureAvailabilitySchema(ctx); err != nil {
				log.Fatalf("ensure HTTPS probe availability schema: %v", err)
			}
		} else {
			if err := requireTables(ctx, dbs.Core, cfg.CoreDB.Database, "node_manual_maintenance"); err != nil {
				log.Fatalf("HTTPS probe enabled but core availability schema is incomplete: %v", err)
			}
			if err := requireTables(ctx, dbs.Runtime, cfg.RuntimeDB.Database, "node_https_probe_state"); err != nil {
				log.Fatalf("HTTPS probe enabled but runtime availability schema is incomplete: %v", err)
			}
		}
		httpsProbeService := service.NewHTTPSProbeService(nodeRepo, workerAdminClient, cfg.HTTPSProbe, cfg.Region.LocalRegion)
		go worker.NewHTTPSProbeMonitor(httpsProbeService, cfg.HTTPSProbe.IntervalSec).Run()
	}

	server := httptransport.NewServer(cfg.App.HTTPAddress, cfg.Auth, cfg.Aggregation, nodeService, regionService, zoneService, routingHandler, aggregationHandler)
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

func requireTables(ctx context.Context, db *sql.DB, schema string, tableNames ...string) error {
	for _, tableName := range tableNames {
		var count int
		if err := db.QueryRowContext(
			ctx,
			`SELECT COUNT(1)
			 FROM information_schema.tables
			 WHERE table_schema = ?
			   AND table_name = ?`,
			schema,
			tableName,
		).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return sql.ErrNoRows
		}
	}
	return nil
}
