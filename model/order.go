package model

type CreateOrderRequest struct {
	Name       string `json:"name" validate:"required,max=100"`
	Email      string `json:"email" validate:"required,email"`
	Phone      string `json:"phone" validate:"required"`
	CategoryId int16  `json:"category_id" validate:"required"`
}

type CreateOrderResponse struct {
	Id          int32  `json:"id"`
	ExternalId  string `json:"external_id"`
	PaymentCode string `json:"payment_code"`
}

type CreateOrderEventMessage struct {
	ID          int32  `json:"id"`
	CategoryID  int16  `json:"category_id"`
	ExternalID  string `json:"external_id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	PaymentCode string `json:"payment_code"`
	ExpiredAt   string `json:"expired_at"`
}

type AssignOrderTicketRowCol struct {
	ID         int32  `json:"id"`
	CategoryId int16  `json:"category_id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
}
