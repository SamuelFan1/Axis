package zone

type Zone struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type ZoneListItem struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Total   int    `json:"total"`
	UpCount int    `json:"up_count"`
	DownCount int  `json:"down_count"`
}
