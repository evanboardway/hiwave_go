package structures

type websocketMessage struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}
