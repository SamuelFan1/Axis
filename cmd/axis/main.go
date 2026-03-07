package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/SamuelFan1/Axis/internal/bootstrap"
	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/repository/mysql"
	"github.com/SamuelFan1/Axis/internal/service"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "service-list":
		if err := runServiceList(); err != nil {
			log.Fatalf("service-list: %v", err)
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func runServiceList() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := bootstrap.OpenDB(cfg.DB)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	nodeRepo := mysql.NewNodeRepository(db)
	nodeService := service.NewNodeService(nodeRepo)
	if err := nodeService.EnsureSchema(context.Background()); err != nil {
		return fmt.Errorf("ensure schema: %w", err)
	}

	items, err := nodeService.List(context.Background())
	if err != nil {
		return fmt.Errorf("list managed nodes: %w", err)
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "UUID\tHOSTNAME\tMANAGEMENT_ADDRESS\tSTATUS\tREGION")
	for _, item := range items {
		fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\t%s\n",
			item.UUID,
			item.Hostname,
			item.ManagementAddress,
			item.Status,
			item.Region,
		)
	}
	writer.Flush()
	return nil
}

func printUsage() {
	fmt.Println("Axis CLI")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  axis service-list")
}
