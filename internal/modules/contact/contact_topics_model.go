package contact

const (
	ObjectList          = "list"
	SubscriptionOptIn   = "opt_in"
	SubscriptionOptOut  = "opt_out"
	maxContactTopicPage = 100
)

type ContactTopic struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	Subscription string  `json:"subscription"`
}

type ListContactTopicsRequest struct {
	Limit  int32
	After  string
	Before string
}

type UpdateContactTopic struct {
	ID           string `json:"id"`
	Subscription string `json:"subscription"`
}

type UpdateContactTopicsRequest []UpdateContactTopic

type ContactTopicListResponse struct {
	Object  string         `json:"object"`
	HasMore bool           `json:"has_more"`
	Data    []ContactTopic `json:"data"`
}

type UpdateContactTopicsResponse struct {
	ID string `json:"id"`
}
