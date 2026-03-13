package main

import (
	"bytes"
	"encoding/json"
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
	case "service-workloads":
		if len(os.Args) < 3 {
			log.Fatalf("service-workloads: uuid is required")
		}
		if err := runServiceWorkloads(os.Args[2]); err != nil {
			log.Fatalf("service-workloads: %v", err)
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
	case "region-create":
		if err := runRegionCreate(os.Args[2:]); err != nil {
			log.Fatalf("region-create: %v", err)
		}
	case "region-delete":
		if len(os.Args) < 3 {
			log.Fatalf("region-delete: uuid is required")
		}
		if err := runRegionDelete(os.Args[2]); err != nil {
			log.Fatalf("region-delete: %v", err)
		}
	case "zone-list":
		if err := runZoneList(); err != nil {
			log.Fatalf("zone-list: %v", err)
		}
	case "zone-create":
		if err := runZoneCreate(os.Args[2:]); err != nil {
			log.Fatalf("zone-create: %v", err)
		}
	case "zone-delete":
		if len(os.Args) < 3 {
			log.Fatalf("zone-delete: uuid is required")
		}
		if err := runZoneDelete(os.Args[2]); err != nil {
			log.Fatalf("zone-delete: %v", err)
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
	region := fs.String("region", "", "node region (continent)")
	zone := fs.String("zone", "", "node zone (country code, e.g. SG, US)")
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
		Zone:              *zone,
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
		{"ZONE", registered.Zone},
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
			item.DNSName,
			item.Status,
			item.Region,
			item.Zone,
		})
	}
	printTable("SERVICE_LIST_RESULT", []string{"UUID", "HOSTNAME", "INTERNAL_IP", "PUBLIC_IP", "DNS_NAME", "STATUS", "REGION", "ZONE"}, rows)
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
		{"DNS_NAME", item.DNSName},
		{"STATUS", item.Status},
		{"REGION", item.Region},
		{"ZONE", item.Zone},
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

type monitoringSnapshotView struct {
	SchemaVersion string                 `json:"schema_version"`
	CollectedAt   time.Time              `json:"collected_at"`
	Sources       []monitoringSourceView `json:"sources"`
}

type monitoringSourceView struct {
	Name        string                 `json:"name"`
	Kind        string                 `json:"kind"`
	Status      string                 `json:"status"`
	CollectedAt time.Time              `json:"collected_at"`
	Summary     map[string]interface{} `json:"summary"`
	Payload     json.RawMessage        `json:"payload"`
	Error       string                 `json:"error"`
}

func runServiceWorkloads(uuidValue string) error {
	client, err := loadAPIClient()
	if err != nil {
		return err
	}

	rawSnapshot, err := client.GetNodeMonitoring(uuidValue)
	if err != nil {
		return err
	}
	if len(rawSnapshot) == 0 || string(rawSnapshot) == "null" {
		printRecord("SERVICE_WORKLOADS_RESULT", [][2]string{
			{"UUID", uuidValue},
			{"RESULT", "no monitoring snapshot"},
		})
		return nil
	}

	var snapshot monitoringSnapshotView
	if err := json.Unmarshal(rawSnapshot, &snapshot); err != nil {
		return fmt.Errorf("decode monitoring snapshot: %w", err)
	}

	printRecord("SERVICE_WORKLOADS_RESULT", [][2]string{
		{"UUID", uuidValue},
		{"SCHEMA_VERSION", snapshot.SchemaVersion},
		{"COLLECTED_AT", formatTime(snapshot.CollectedAt)},
		{"SOURCE_COUNT", fmt.Sprintf("%d", len(snapshot.Sources))},
	})

	rows := make([][]string, 0, len(snapshot.Sources))
	for _, source := range snapshot.Sources {
		rows = append(rows, []string{
			source.Name,
			source.Kind,
			source.Status,
			formatTime(source.CollectedAt),
		})
	}
	printTable("MONITORING_SOURCES", []string{"NAME", "KIND", "STATUS", "COLLECTED_AT"}, rows)

	for _, source := range snapshot.Sources {
		if len(source.Summary) > 0 {
			summaryFields := make([][2]string, 0, len(source.Summary)+4)
			summaryFields = append(summaryFields,
				[2]string{"NAME", source.Name},
				[2]string{"KIND", source.Kind},
				[2]string{"STATUS", source.Status},
				[2]string{"COLLECTED_AT", formatTime(source.CollectedAt)},
			)
			for key, value := range source.Summary {
				summaryFields = append(summaryFields, [2]string{strings.ToUpper(key), fmt.Sprintf("%v", value)})
			}
			printRecord(strings.ToUpper(strings.ReplaceAll(source.Name, "-", "_"))+"_SUMMARY", summaryFields)
		}
		if source.Error != "" {
			printRecord(strings.ToUpper(strings.ReplaceAll(source.Name, "-", "_"))+"_ERROR", [][2]string{
				{"NAME", source.Name},
				{"ERROR", source.Error},
			})
		}
		if len(source.Payload) > 0 && string(source.Payload) != "null" {
			fmt.Printf("%s_PAYLOAD\n%s\n", strings.ToUpper(strings.ReplaceAll(source.Name, "-", "_")), prettyJSON(source.Payload))
		}
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
			item.UUID,
			item.Name,
			fmt.Sprintf("%d", item.ZoneNum),
		})
	}
	printTable("REGION_LIST_RESULT", []string{"UUID", "NAME", "ZONE_NUM"}, rows)
	return nil
}

func runRegionCreate(args []string) error {
	fs := flag.NewFlagSet("region-create", flag.ContinueOnError)
	name := fs.String("name", "", "region name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	client, err := loadAPIClient()
	if err != nil {
		return err
	}
	uuid, regionName, err := client.CreateRegion(*name)
	if err != nil {
		return err
	}
	printRecord("REGION_CREATE_RESULT", [][2]string{
		{"UUID", uuid},
		{"NAME", regionName},
	})
	return nil
}

func runRegionDelete(uuidValue string) error {
	client, err := loadAPIClient()
	if err != nil {
		return err
	}
	if err := client.DeleteRegion(uuidValue); err != nil {
		return err
	}
	printRecord("REGION_DELETE_RESULT", [][2]string{
		{"UUID", uuidValue},
		{"RESULT", "deleted"},
	})
	return nil
}

func runZoneList() error {
	client, err := loadAPIClient()
	if err != nil {
		return err
	}
	items, err := client.ListZones()
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.UUID,
			item.Name,
			fmt.Sprintf("%d", item.Total),
			fmt.Sprintf("%d", item.UpCount),
			fmt.Sprintf("%d", item.DownCount),
		})
	}
	printTable("ZONE_LIST_RESULT", []string{"UUID", "NAME", "TOTAL", "UP", "DOWN"}, rows)
	return nil
}

func runZoneCreate(args []string) error {
	fs := flag.NewFlagSet("zone-create", flag.ContinueOnError)
	name := fs.String("name", "", "zone name (e.g. SG, CN)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	client, err := loadAPIClient()
	if err != nil {
		return err
	}
	uuid, zoneName, err := client.CreateZone(*name)
	if err != nil {
		return err
	}
	printRecord("ZONE_CREATE_RESULT", [][2]string{
		{"UUID", uuid},
		{"NAME", zoneName},
	})
	return nil
}

func runZoneDelete(uuidValue string) error {
	client, err := loadAPIClient()
	if err != nil {
		return err
	}
	if err := client.DeleteZone(uuidValue); err != nil {
		return err
	}
	printRecord("ZONE_DELETE_RESULT", [][2]string{
		{"UUID", uuidValue},
		{"RESULT", "deleted"},
	})
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
	fmt.Println("  axis service-register --hostname <name> --management-address <addr> --region <region> --zone <zone> [--status up] [--uuid <uuid>]")
	fmt.Println("  axis service-list")
	fmt.Println("  axis service-show <uuid>")
	fmt.Println("  axis service-workloads <uuid>")
	fmt.Println("  axis service-delete <uuid>")
	fmt.Println("  axis service-up <uuid>")
	fmt.Println("  axis service-down <uuid>")
	fmt.Println("  axis region-list")
	fmt.Println("  axis region-create --name <name>")
	fmt.Println("  axis region-delete <uuid>")
	fmt.Println("  axis zone-list")
	fmt.Println("  axis zone-create --name <name>")
	fmt.Println("  axis zone-delete <uuid>")
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

func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, raw, "", "  "); err != nil {
		return string(raw)
	}
	return indented.String()
}

func extractInternalIP(managementAddress string) string {
	host, _, err := net.SplitHostPort(managementAddress)
	if err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	return managementAddress
}
