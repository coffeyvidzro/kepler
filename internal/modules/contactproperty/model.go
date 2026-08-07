package contactproperty

import "time"

const ObjectContactProperty = "contact_property"

type Property struct {
	ID            string    `json:"id"`
	TeamID        string    `json:"team_id"`
	Key           string    `json:"key"`
	Type          string    `json:"type"`
	FallbackValue any       `json:"fallback_value,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateRequest struct {
	Key           string `json:"key"`
	Type          string `json:"type"`
	FallbackValue any    `json:"fallback_value,omitempty"`
}

type UpdateRequest struct {
	FallbackValue any `json:"fallback_value"`
}

type ListRequest struct {
	Limit  int32
	After  string
	Before string
}

type MutationResponse struct {
	Object string `json:"object"`
	ID     string `json:"id"`
}

type DeleteResponse struct {
	Object  string `json:"object"`
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

type ResourceResponse struct {
	Object        string    `json:"object"`
	ID            string    `json:"id"`
	Key           string    `json:"key"`
	Type          string    `json:"type"`
	FallbackValue any       `json:"fallback_value,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type ListResponse struct {
	Object  string             `json:"object"`
	HasMore bool               `json:"has_more"`
	Data    []ResourceResponse `json:"data"`
}

func (property Property) MutationResponse() MutationResponse {
	return MutationResponse{Object: ObjectContactProperty, ID: property.ID}
}

func (property Property) DeleteResponse() DeleteResponse {
	return DeleteResponse{Object: ObjectContactProperty, ID: property.ID, Deleted: true}
}

func (property Property) ResourceResponse() ResourceResponse {
	return ResourceResponse{
		Object:        ObjectContactProperty,
		ID:            property.ID,
		Key:           property.Key,
		Type:          property.Type,
		FallbackValue: property.FallbackValue,
		CreatedAt:     property.CreatedAt,
	}
}

func ResourceResponses(properties []Property) []ResourceResponse {
	responses := make([]ResourceResponse, len(properties))
	for index, property := range properties {
		responses[index] = property.ResourceResponse()
	}
	return responses
}
