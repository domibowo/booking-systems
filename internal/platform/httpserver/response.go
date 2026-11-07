package httpserver

import "github.com/gin-gonic/gin"

// Response is the single envelope shape returned by every endpoint, success
// or failure: {"status": <http status>, "message": "...", "data": {...}}.
type Response struct {
	Status  int         `json:"status" example:"200"`
	Message string      `json:"message" example:"success"`
	Data    interface{} `json:"data"`
}

// JSON writes the envelope with the given HTTP status, message, and payload.
// A nil data is normalized to an empty object so "data" is never null.
func JSON(c *gin.Context, status int, message string, data interface{}) {
	if data == nil {
		data = gin.H{}
	}
	c.JSON(status, Response{Status: status, Message: message, Data: data})
}
