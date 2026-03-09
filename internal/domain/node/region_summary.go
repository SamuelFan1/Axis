package node

type RegionSummary struct {
	Region    string
	Total     int
	UpCount   int
	DownCount int
}

type RegionZoneSummary struct {
	Region    string `json:"region"`
	Zone      string `json:"zone"`
	Total     int    `json:"total"`
	UpCount   int    `json:"up_count"`
	DownCount int    `json:"down_count"`
}
