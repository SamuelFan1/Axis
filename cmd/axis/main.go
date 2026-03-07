package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	httptransport "github.com/SamuelFan1/Axis/internal/transport/http"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "service-register":
		if err := runServiceRegister(os.Args[2:]); err != nil {
			log.Fatalf("service-register: %v", err)
		}
	case "service-list":
		if err := runServiceList(); err != nil {
			log.Fatalf("service-list: %v", err)
		}
	case "service-show":
		if len(os.Args) < 3 {
			log.Fatalf("service-show: uuid is required")
		}
		if err := runServiceShow(os.Args[2]); err != nil {
			log.Fatalf("service-show: %v", err)
		}
	case "service-delete":
		if len(os.Args) < 3 {
			log.Fatalf("service-delete: uuid is required")
		}
		if err := runServiceDelete(os.Args[2]); err != nil {
			log.Fatalf("service-delete: %v", err)
		}
	case "service-up":
		if len(os.Args) < 3 {
			log.Fatalf("service-up: uuid is required")
		}
		if err := runServiceSetStatus(os.Args[2], node.StatusUp); err != nil {
			log.Fatalf("service-up: %v", err)
		}
	case "service-down":
		if len(os.Args) < 3 {
			log.Fatalf("service-down: uuid is required")
		}
		if err := runServiceSetStatus(os.Args[2], node.StatusDown); err != nil {
			log.Fatalf("service-down: %v", err)
		}
	case "region-list":
		if err := runRegionList(); err != nil {
			log.Fatalf("region-list: %v", err)
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func runServiceRegister(args []string) error {
	fs := flag.NewFlagSet("service-register", flag.ContinueOnError)
	uuidValue := fs.String("uuid", "", "existing node uuid (optional)")
	hostname := fs.String("hostname", "", "node hostname")
	managementAddress := fs.String("management-address", "", "node management address")
	region := fs.String("region", "", "node region")
	status := fs.String("status", node.StatusUp, "node status: up or down")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := loadAPIClient()
	if err != nil {
		return err
	}

	registered, err := client.RegisterNode(httptransport.RegisterNodeRequest{
		UUID:              *uuidValue,
		Hostname:          *hostname,
		ManagementAddress: *managementAddress,
		Region:            *region,
		Status:            *status,
	})
	if err != nil {
		return err
	}

	printRecord("SERVICE_REGISTER_RESULT", [][2]string{
		{"UUID", registered.UUID},
		{"HOSTNAME", registered.Hostname},
		{"INTERNAL_IP", extractInternalIP(registered.ManagementAddress)},
		{"STATUS", registered.Status},
		{"REGION", registered.Region},
	})
	return nil
}

func runServiceList() error {
	client, err := loadAPIClient()
	if err != nil {
		return err
	}

	items, err := client.ListNodes()
	if err != nil {
		return fmt.Errorf("list managed nodes: %w", err)
	}

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		internalIP := item.InternalIP
		if internalIP == "" {
			internalIP = extractInternalIP(item.ManagementAddress)
		}
		rows = append(rows, []string{
			item.UUID,
			item.Hostname,
			internalIP,
			item.PublicIP,
			item.Status,
			item.Region,
		})
	}
	printTable("SERVICE_LIST_RESULT", []string{"UUID", "HOSTNAME", "INTERNAL_IP", "PUBLIC_IP", "STATUS", "REGION"}, rows)
	return nil
}

func runServiceShow(uuidValue string) error {
	client, err := loadAPIClient()
	if err != nil {
		return err
	}

	item, err := client.GetNode(uuidValue)
	if err != nil {
		return err
	}

	internalIP := item.InternalIP
	if internalIP == "" {
		internalIP = extractInternalIP(item.ManagementAddress)
	}

	fields := [][2]string{
		{"UUID", item.UUID},
		{"HOSTNAME", item.Hostname},
		{"INTERNAL_IP", internalIP},
		{"PUBLIC_IP", item.PublicIP},
		{"STATUS", item.Status},
		{"REGION", item.Region},
		{"CPU_CORES", fmt.Sprintf("%d cores", item.CPUCores)},
		{"CPU_USAGE_PERCENT", fmt.Sprintf("%.1f%%", item.CPUUsagePercent)},
		{"MEMORY_TOTAL_GB", fmt.Sprintf("%.2f GB", item.MemoryTotalGB)},
		{"MEMORY_USED_GB", fmt.Sprintf("%.2f GB", item.MemoryUsedGB)},
		{"MEMORY_USAGE_PERCENT", fmt.Sprintf("%.1f%%", item.MemoryUsagePercent)},
		{"SWAP_TOTAL_GB", fmt.Sprintf("%.2f GB", item.SwapTotalGB)},
		{"SWAP_USED_GB", fmt.Sprintf("%.2f GB", item.SwapUsedGB)},
		{"SWAP_USAGE_PERCENT", fmt.Sprintf("%.1f%%", item.SwapUsagePercent)},
		{"DISK_USAGE_PERCENT", fmt.Sprintf("%.1f%%", item.DiskUsagePercent)},
		{"LAST_SEEN_AT", item.LastSeenAt.Format("2006-01-02 15:04:05")},
		{"LAST_REPORTED_AT", formatTime(item.LastReportedAt)},
	}
	printRecord("SERVICE_SHOW_RESULT", fields)

	if len(item.DiskDetails) > 0 {
		diskRows := make([][]string, 0, len(item.DiskDetails))
		for _, d := range item.DiskDetails {
			diskRows = append(diskRows, []string{
				d.MountPoint,
				d.Filesystem,
				fmt.Sprintf("%.2f GB", d.TotalGB),
				fmt.Sprintf("%.2f GB", d.UsedGB),
				fmt.Sprintf("%.1f%%", d.UsagePercent),
			})
		}
		printTable("DISK_DETAILS", []string{"MOUNT_POINT", "FILESYSTEM", "TOTAL_GB", "USED_GB", "USAGE_PERCENT"}, diskRows)
	}
	return nil
}

func runServiceDelete(uuidValue string) error {
	client, err := loadAPIClient()
	if err != nil {
		return err
	}

	if err := client.DeleteNode(uuidValue); err != nil {
		return err
	}
	printRecord("SERVICE_DELETE_RESULT", [][2]string{
		{"UUID", uuidValue},
		{"RESULT", "deleted"},
	})
	return nil
}

func runServiceSetStatus(uuidValue string, status string) error {
	client, err := loadAPIClient()
	if err != nil {
		return err
	}

	item, err := client.UpdateNodeStatus(uuidValue, status)
	if err != nil {
		return err
	}

	printRecord("SERVICE_STATUS_RESULT", [][2]string{
		{"UUID", item.UUID},
		{"HOSTNAME", item.Hostname},
		{"STATUS", item.Status},
		{"REGION", item.Region},
	})
	return nil
}

func runRegionList() error {
	client, err := loadAPIClient()
	if err != nil {
		return err
	}

	items, err := client.ListRegions()
	if err != nil {
		return err
	}

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.Region,
			fmt.Sprintf("%d", item.Total),
			fmt.Sprintf("%d", item.UpCount),
			fmt.Sprintf("%d", item.DownCount),
		})
	}
	printTable("REGION_LIST_RESULT", []string{"REGION", "TOTAL", "UP", "DOWN"}, rows)
	return nil
}

func loadAPIClient() (*httptransport.Client, error) {
	cfg, err := config.LoadCLIAuth()
	if err != nil {
		return nil, fmt.Errorf("load cli auth: %w", err)
	}
	return httptransport.NewClient(*cfg), nil
}

func printUsage() {
	fmt.Println("Axis CLI")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  axis service-register --hostname <name> --management-address <addr> --region <region> [--status up] [--uuid <uuid>]")
	fmt.Println("  axis service-list")
	fmt.Println("  axis service-show <uuid>")
	fmt.Println("  axis service-delete <uuid>")
	fmt.Println("  axis service-up <uuid>")
	fmt.Println("  axis service-down <uuid>")
	fmt.Println("  axis region-list")
}

func printTable(title string, headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}

	for _, row := range rows {
		for i := range headers {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	totalInnerWidth := 1
	for _, width := range widths {
		totalInnerWidth += width + 3
	}

	titleBorder := "+" + strings.Repeat("-", totalInnerWidth) + "+"
	fmt.Println(titleBorder)
	fmt.Printf("| %-*s |\n", totalInnerWidth-2, title)
	fmt.Println(titleBorder)

	border := buildBorder(widths)
	fmt.Println(border)
	fmt.Println(formatRow(headers, widths))
	fmt.Println(border)
	for _, row := range rows {
		fmt.Println(formatRow(row, widths))
	}
	fmt.Println(border)
}

func printRecord(title string, fields [][2]string) {
	widths := []int{len("FIELD"), len("VALUE")}
	for _, field := range fields {
		if len(field[0]) > widths[0] {
			widths[0] = len(field[0])
		}
		if len(field[1]) > widths[1] {
			widths[1] = len(field[1])
		}
	}

	totalInnerWidth := 1
	for _, width := range widths {
		totalInnerWidth += width + 3
	}

	titleBorder := "+" + strings.Repeat("-", totalInnerWidth) + "+"
	fmt.Println(titleBorder)
	fmt.Printf("| %-*s |\n", totalInnerWidth-2, title)
	fmt.Println(titleBorder)

	border := buildBorder(widths)
	fmt.Println(formatRow([]string{"FIELD", "VALUE"}, widths))
	fmt.Println(border)
	for _, field := range fields {
		fmt.Println(formatRow([]string{field[0], field[1]}, widths))
	}
	fmt.Println(border)
}

func buildBorder(widths []int) string {
	var builder strings.Builder
	builder.WriteString("+")
	for _, width := range widths {
		builder.WriteString(strings.Repeat("-", width+2))
		builder.WriteString("+")
	}
	return builder.String()
}

func formatRow(values []string, widths []int) string {
	var builder strings.Builder
	builder.WriteString("|")
	for i, width := range widths {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		builder.WriteString(" ")
		builder.WriteString(value)
		builder.WriteString(strings.Repeat(" ", width-len(value)+1))
		builder.WriteString("|")
	}
	return builder.String()
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format("2006-01-02 15:04:05")
}

func extractInternalIP(managementAddress string) string {
	host, _, err := net.SplitHostPort(managementAddress)
	if err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	return managementAddress
}
