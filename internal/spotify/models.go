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

type AudioFeatures struct {
	ID               string  `json:"id"`
	Danceability     float64 `json:"danceability"`
	Energy           float64 `json:"energy"`
	Valence          float64 `json:"valence"`
	Instrumentalness float64 `json:"instrumentalness"`
}

type AudioFeaturesResponse struct {
	AudioFeatures []AudioFeatures `json:"audio_features"`
}
