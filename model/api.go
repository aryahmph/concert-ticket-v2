package model

type ErrorResponse struct {
	Error string `json:"error"`
	Data  any    `json:"data,omitempty"`
}
