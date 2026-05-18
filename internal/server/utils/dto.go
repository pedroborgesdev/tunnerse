package utils

type RegisterRequest struct {
	Name string `json:"name" binding:"required"`
}
