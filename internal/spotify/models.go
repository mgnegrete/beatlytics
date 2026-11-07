package spotify

type RecentlyPlayedResponse struct {
	Items []TrackItem `json:"items"`
}

type TrackItem struct {
	Track    Track  `json:"track"`
	PlayedAt string `json:"played_at"`
}

type Track struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Artists []Artist `json:"artists"`
}

type Artist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
