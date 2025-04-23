package model

type PaymentCallbackRequest struct {
	ExternalId string `json:"external_id" validate:"required"`
}
