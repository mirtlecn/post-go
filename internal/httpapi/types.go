package httpapi

// Ordered response payloads to match legacy JSON field order.

type ItemResponse struct {
	SURL    string `json:"surl"`
	Path    string `json:"path"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Created string `json:"created"`
	TTL     *int64 `json:"ttl"`
	Content string `json:"content"`
}

type CreateResponse struct {
	SURL        string `json:"surl"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Created     string `json:"created"`
	Content     string `json:"content"`
	TTL         any    `json:"ttl"`
	Overwritten string `json:"overwritten,omitempty"`
	Warning     string `json:"warning,omitempty"`
}

type DeleteResponse struct {
	Deleted string `json:"deleted"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Created string `json:"created"`
	Content string `json:"content"`
}
