package model

type CategoryResponse struct {
	Id       int16  `json:"id"`
	Name     string `json:"name"`
	Price    int32  `json:"price"`
	Quantity int32  `json:"quantity"`
}

type ListCategoriesResponse struct {
	Categories []CategoryResponse `json:"categories"`
}

type IncrementCategoryQuantityEventMessage struct {
	ID       int16 `json:"id"`
	Quantity int32 `json:"quantity"`
}
