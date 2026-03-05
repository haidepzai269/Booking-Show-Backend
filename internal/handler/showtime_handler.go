package handler

import (
	"net/http"
	"strconv"

	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/gin-gonic/gin"
)

type ShowtimeHandler struct {
	ShowtimeService *service.ShowtimeService
}

func NewShowtimeHandler() *ShowtimeHandler {
	return &ShowtimeHandler{
		ShowtimeService: &service.ShowtimeService{},
	}
}

// Lấy danh sách suất chiếu của 1 bộ phim
func (h *ShowtimeHandler) GetShowtimes(c *gin.Context) {
	movieID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid movie ID"})
		return
	}

	showtimes, err := h.ShowtimeService.GetShowtimesByMovie(movieID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": showtimes})
}

// Lấy chi tiết 1 suất chiếu (kèm Movie, Cinema, Room)
func (h *ShowtimeHandler) GetShowtime(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid showtime ID"})
		return
	}

	showtime, err := h.ShowtimeService.GetShowtime(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "showtime not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": showtime})
}

func (h *AdminHandler) CreateShowtime(c *gin.Context) {
	var req service.CreateShowtimeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error(), "code": "INVALID_INPUT"})
		return
	}

	showtimeService := &service.ShowtimeService{}
	showtime, err := showtimeService.CreateShowtime(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": showtime})
}
