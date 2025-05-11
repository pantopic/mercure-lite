package mercurelite

type subscription struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Topic      string         `json:"topic"`
	Subscriber string         `json:"subscriber"`
	Active     bool           `json:"active"`
	Payload    map[string]any `json:"payload"`
}

type subscriptionList struct {
	Context       string         `json:"@context"`
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	LastEventID   string         `json:"lastEventID"`
	Subscriptions []subscription `json:"subscriptions"`
}
