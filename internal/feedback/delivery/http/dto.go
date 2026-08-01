package http

import "time"

type CreateFeedbackResponse struct {
	ID        string    `json:"id" example:"9b42f4de-22cb-4f3a-9a9c-6e46139fd799"`
	Type      string    `json:"type" example:"bug"`
	Email     string    `json:"email" example:"user@example.com"`
	Content   string    `json:"content" example:"There is a bug on the checkout page."`
	MediaURL  *string   `json:"media_url,omitempty" example:"https://res.cloudinary.com/.../file.png"`
	CreatedAt time.Time `json:"created_at" example:"2026-06-19T07:44:30Z"`
}

type FeedbackResponse struct {
	ID        string    `json:"id" example:"9b42f4de-22cb-4f3a-9a9c-6e46139fd799"`
	Type      string    `json:"type" example:"product_improvement"`
	Email     string    `json:"email" example:"user@example.com"`
	Content   string    `json:"content" example:"We need dark mode!"`
	MediaURL  *string   `json:"media_url,omitempty" example:"https://res.cloudinary.com/.../file.png"`
	CreatedAt time.Time `json:"created_at" example:"2026-06-19T07:44:30Z"`
}

type ErrorResponse struct {
	Error   string `json:"error" example:"Bad Request"`
	Message string `json:"message" example:"MIME type is not allowed"`
}
