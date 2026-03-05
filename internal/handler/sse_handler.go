package handler

import (
	"io"
	"strconv"

	"github.com/booking-show/booking-show-api/pkg/sse"
	"github.com/gin-gonic/gin"
)

type SSEHandler struct{}

func NewSSEHandler() *SSEHandler {
	return &SSEHandler{}
}

func (h *SSEHandler) Stream(c *gin.Context) {
	showtimeID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.String(400, "invalid showtime id")
		return
	}

	hub := sse.GetHub(showtimeID)
	client := &sse.Client{
		Channel: make(chan string, 10),
	}
	hub.AddClient(client)

	defer hub.RemoveClient(client)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-client.Channel:
			if !ok {
				return false
			}
			c.SSEvent("message", msg)
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}
