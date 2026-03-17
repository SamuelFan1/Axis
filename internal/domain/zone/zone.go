package zone

type Zone struct {
	UUID       string `json:"uuid"`
	RegionUUID string `json:"region_uuid"`
	Name       string `json:"name"`
}

type ZoneListItem struct {
	UUID       string `json:"uuid"`
	RegionUUID string `json:"region_uuid"`
	RegionName string `json:"region_name"`
	Name       string `json:"name"`
	Total      int    `json:"total"`
	UpCount    int    `json:"up_count"`
	DownCount  int    `json:"down_count"`
}
