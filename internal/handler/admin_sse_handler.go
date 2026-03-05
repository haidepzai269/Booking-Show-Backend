package handler

import (
	"io"

	"github.com/booking-show/booking-show-api/pkg/sse"
	"github.com/gin-gonic/gin"
)

type AdminSSEHandler struct{}

func NewAdminSSEHandler() *AdminSSEHandler {
	return &AdminSSEHandler{}
}

// Stream — GET /api/v1/admin/notifications/stream
// Admin client kết nối SSE để nhận thông báo real-time
func (h *AdminSSEHandler) Stream(c *gin.Context) {
	hub := sse.GetAdminHub()
	client := &sse.Client{
		Channel: make(chan string, 20),
	}
	hub.AddClient(client)
	defer hub.RemoveClient(client)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	// Gửi ping đầu tiên để xác nhận kết nối
	c.SSEvent("ping", "connected")
	c.Writer.Flush()

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-client.Channel:
			if !ok {
				return false
			}
			c.SSEvent("notification", msg)
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}
