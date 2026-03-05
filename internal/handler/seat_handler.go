package handler

import (
	"io"
	"net/http"
	"strconv"

	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/booking-show/booking-show-api/pkg/sse"
	"github.com/gin-gonic/gin"
)

type SeatHandler struct {
	SeatService *service.SeatService
}

func NewSeatHandler() *SeatHandler {
	return &SeatHandler{
		SeatService: &service.SeatService{},
	}
}

func (h *SeatHandler) GetSeats(c *gin.Context) {
	showtimeID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid showtime ID"})
		return
	}

	seats, err := h.SeatService.GetSeats(showtimeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Transform sang flat DTO cho frontend dễ xài
	type SeatDTO struct {
		ID         int    `json:"id"`
		ShowtimeID int    `json:"showtime_id"`
		SeatID     int    `json:"seat_id"`
		Status     string `json:"status"`
		Price      int    `json:"price"`
		RoomID     int    `json:"room_id"`
		RowChar    string `json:"row_char"`
		SeatNumber int    `json:"seat_number"`
		Type       string `json:"type"`
	}

	var resp []SeatDTO
	for _, s := range seats {
		resp = append(resp, SeatDTO{
			ID:         s.ID,
			ShowtimeID: s.ShowtimeID,
			SeatID:     s.SeatID,
			Status:     string(s.Status),
			Price:      s.Price,
			RoomID:     s.Seat.RoomID,
			RowChar:    s.Seat.RowChar,
			SeatNumber: s.Seat.SeatNumber,
			Type:       string(s.Seat.Type),
		})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// GetSeatsStream thiết lập kết nối SSE để gửi cập nhật ghế thời gian thực
func (h *SeatHandler) GetSeatsStream(c *gin.Context) {
	showtimeID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid showtime ID"})
		return
	}

	client := &sse.Client{
		Channel: make(chan string),
	}
	hub := sse.GetHub(showtimeID)
	hub.AddClient(client)
	defer hub.RemoveClient(client)

	// Thiết lập headers cho SSE
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	c.Stream(func(w io.Writer) bool {
		if msg, ok := <-client.Channel; ok {
			c.SSEvent("message", msg)
			return true
		}
		return false
	})
}

func (h *SeatHandler) LockSeats(c *gin.Context) {
	var req service.LockSeatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	userID := c.GetInt("userID")

	if err := h.SeatService.LockSeat(req, userID); err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": err.Error(), "code": "SEAT_LOCK_FAILED"})
		return
	}

	// Trigger SSE Event (TODO)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": "Seats locked successfully for 10 minutes"})
}

// User tự huỷ UnlockSeats
type UnlockSeatReq struct {
	ShowtimeID int   `json:"showtime_id" binding:"required"`
	SeatIDs    []int `json:"seat_ids" binding:"required,min=1"`
}

func (h *SeatHandler) UnlockSeats(c *gin.Context) {
	var req UnlockSeatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	userID := c.GetInt("userID")
	if err := h.SeatService.UnlockSeats(req.ShowtimeID, userID, req.SeatIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Trigger SSE Event (TODO)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": "Seats unlocked"})
}

func (h *AdminHandler) InitSeats(c *gin.Context) {
	roomID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid room ID"})
		return
	}

	var req struct {
		ShowtimeID int `json:"showtime_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	svc := &service.SeatService{}
	if err := svc.InitSeats(roomID, req.ShowtimeID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": "Seats initialized locally"})
}
