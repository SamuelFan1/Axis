package main

import (
	"context"
	"log"

	"github.com/SamuelFan1/Axis/internal/bootstrap"
	"github.com/SamuelFan1/Axis/internal/config"
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
	nodeService := service.NewNodeService(nodeRepo)
	if err := nodeService.EnsureSchema(context.Background()); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	nodeMonitor := worker.NewNodeMonitor(
		nodeService,
		cfg.App.NodeTimeoutSec,
		cfg.App.NodeMonitorIntervalSec,
	)
	go nodeMonitor.Run()

	server := httptransport.NewServer(cfg.App.HTTPAddress, cfg.Auth, nodeService)
	if err := server.Run(); err != nil {
		log.Fatalf("run http server: %v", err)
	}
}
