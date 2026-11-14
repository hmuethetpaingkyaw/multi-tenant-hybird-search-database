package response

// ErrorResponse is the body of every error: {"error": "<message>"}.
type ErrorResponse struct {
	Error string `json:"error"`
}

// StatusResponse is the body of the health probes: {"status": "<state>"}.
type StatusResponse struct {
	Status string `json:"status"`
}
