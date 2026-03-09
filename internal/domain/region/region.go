package region

type Region struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type RegionListItem struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	ZoneNum int    `json:"zone_num"`
}
