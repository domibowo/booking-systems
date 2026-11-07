package httpserver

// ErrorResponse is the consistent error shape returned by every endpoint.
type ErrorResponse struct {
	Error string `json:"error"`
}
