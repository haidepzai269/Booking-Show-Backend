package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/gin-gonic/gin"
)

type CinemaHandler struct {
	CinemaService *service.CinemaService
}

func NewCinemaHandler() *CinemaHandler {
	return &CinemaHandler{
		CinemaService: &service.CinemaService{},
	}
}

func (h *CinemaHandler) ListCinemas(c *gin.Context) {
	var userLat, userLng *float64

	// Parse query params lat/lng (tùy chọn, nếu không có sẽ trả danh sách bình thường)
	if latStr := c.Query("lat"); latStr != "" {
		if latVal, err := strconv.ParseFloat(latStr, 64); err == nil {
			userLat = &latVal
		}
	}
	if lngStr := c.Query("lng"); lngStr != "" {
		if lngVal, err := strconv.ParseFloat(lngStr, 64); err == nil {
			userLng = &lngVal
		}
	}

	cinemas, err := h.CinemaService.ListCinemasNearby(userLat, userLng)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cinemas})
}

func (h *CinemaHandler) GetCinema(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid cinema ID"})
		return
	}

	cinema, err := h.CinemaService.GetCinema(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error(), "code": "NOT_FOUND"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cinema})
}

// GetCinemaMovies - GET /cinemas/:id/movies?date=2026-03-02
func (h *CinemaHandler) GetCinemaMovies(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid cinema ID"})
		return
	}

	// Parse ngày, mặc định là hôm nay (giờ Việt Nam)
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	date := time.Now().In(loc)
	if dateStr := c.Query("date"); dateStr != "" {
		if parsed, parseErr := time.ParseInLocation("2006-01-02", dateStr, loc); parseErr == nil {
			date = parsed
		}
	}

	movies, err := h.CinemaService.GetCinemaMovies(id, date)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": movies})
}

// GetCinemaRooms - GET /cinemas/:id/rooms
func (h *CinemaHandler) GetCinemaRooms(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid cinema ID"})
		return
	}

	rooms, err := h.CinemaService.GetCinemaRooms(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rooms})
}

// Bổ sung AdminHandler Funcs

type CreateCinemaReq struct {
	Name      string   `json:"name" binding:"required"`
	Address   string   `json:"address" binding:"required"`
	City      string   `json:"city"`
	ImageURL  string   `json:"image_url"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

func (h *AdminHandler) CreateCinema(c *gin.Context) {
	var req CreateCinemaReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	cinemaSvc := &service.CinemaService{}
	cinema, err := cinemaSvc.CreateCinema(service.CreateCinemaServiceReq{
		Name:      req.Name,
		Address:   req.Address,
		City:      req.City,
		ImageURL:  req.ImageURL,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": cinema})
}

func (h *AdminHandler) CreateRoom(c *gin.Context) {
	cinemaID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid cinema ID"})
		return
	}

	var req service.RoomReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	cinemaSvc := &service.CinemaService{}
	room, err := cinemaSvc.CreateRoom(cinemaID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": room})
}

func (h *AdminHandler) ListAdminCinemas(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	q := c.Query("q")

	cinemaSvc := &service.CinemaService{}
	cinemas, total, err := cinemaSvc.ListAdminCinemas(page, limit, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    cinemas,
		"meta": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

func (h *AdminHandler) UpdateCinema(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid cinema ID"})
		return
	}

	var req service.UpdateCinemaReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	cinemaSvc := &service.CinemaService{}
	cinema, err := cinemaSvc.UpdateCinema(id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": cinema})
}

func (h *AdminHandler) DeleteCinema(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid cinema ID"})
		return
	}

	cinemaSvc := &service.CinemaService{}
	if err := cinemaSvc.DeleteCinema(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Cinema deleted successfully"})
}

func (h *AdminHandler) ListAdminRoomsByCinema(c *gin.Context) {
	cinemaID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid cinema ID"})
		return
	}

	cinemaSvc := &service.CinemaService{}
	rooms, err := cinemaSvc.GetCinemaRooms(cinemaID) // Reuse GetCinemaRooms hoặc có thể custom nếu cần
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": rooms})
}

func (h *AdminHandler) DeleteRoom(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid room ID"})
		return
	}

	cinemaSvc := &service.CinemaService{}
	if err := cinemaSvc.DeleteRoom(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Room deleted successfully"})
}
