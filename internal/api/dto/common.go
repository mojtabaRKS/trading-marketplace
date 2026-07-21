// Package dto holds the HTTP request and response types (data transfer objects)
// and pure converters from persistence models. It has no HTTP or business logic.
package dto

// ErrorResponse is the standard JSON error body returned by the API.
type ErrorResponse struct {
	Error string `json:"error" example:"item not found"`
}

// StatusResponse is a simple status acknowledgement.
type StatusResponse struct {
	Status string `json:"status" example:"cancelled"`
}

// HealthResponse is the liveness probe body.
type HealthResponse struct {
	Status string `json:"status" example:"ok"`
}
