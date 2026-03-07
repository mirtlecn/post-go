package httpapi

// Ordered response payloads to match legacy JSON field order.

type ItemResponse struct {
	SURL    string `json:"surl"`
	Path    string `json:"path"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

type CreateResponse struct {
	SURL       string `json:"surl"`
	Path       string `json:"path"`
	Type       string `json:"type"`
	Content    string `json:"content"`
	ExpiresIn  any    `json:"expires_in"`
	Overwritten string `json:"overwritten,omitempty"`
	Warning    string `json:"warning,omitempty"`
}

type DeleteResponse struct {
	Deleted string `json:"deleted"`
	Type    string `json:"type"`
	Content string `json:"content"`
}
