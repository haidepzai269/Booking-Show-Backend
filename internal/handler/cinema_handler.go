package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/booking-show/booking-show-api/internal/service"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/booking-show/booking-show-api/pkg/sse"
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

	// Active Invalidation
	if redispkg.Client != nil {
		iter := redispkg.Client.Scan(redispkg.Ctx, 0, "admin:cinemas:list:*", 0).Iterator()
		var keysToDelete []string
		for iter.Next(redispkg.Ctx) {
			keysToDelete = append(keysToDelete, iter.Val())
		}
		if len(keysToDelete) > 0 {
			redispkg.Client.Del(redispkg.Ctx, keysToDelete...)
			log.Printf("[Cache INVALIDATED] %d keys cleared after CreateCinema\n", len(keysToDelete))
		}
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

	// Active Invalidation
	if redispkg.Client != nil {
		iter := redispkg.Client.Scan(redispkg.Ctx, 0, "admin:cinemas:list:*", 0).Iterator()
		var keysToDelete []string
		for iter.Next(redispkg.Ctx) {
			keysToDelete = append(keysToDelete, iter.Val())
		}
		if len(keysToDelete) > 0 {
			redispkg.Client.Del(redispkg.Ctx, keysToDelete...)
			log.Printf("[Cache INVALIDATED] %d keys cleared after CreateRoom\n", len(keysToDelete))
		}
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": room})
}

func (h *AdminHandler) ListAdminCinemas(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	q := c.Query("q")

	cacheKey := "admin:cinemas:list:" + strconv.Itoa(page) + ":" + strconv.Itoa(limit) + ":" + q

	// Get from Cache
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result(); err == nil {
			var cachedResult map[string]interface{}
			if err := json.Unmarshal([]byte(cached), &cachedResult); err == nil {
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"data":    cachedResult["data"],
					"meta":    cachedResult["meta"],
					"cached":  true,
				})
				return
			}
		}
	}

	cinemaSvc := &service.CinemaService{}
	cinemas, total, err := cinemaSvc.ListAdminCinemas(page, limit, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	result := gin.H{
		"data": cinemas,
		"meta": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	}

	// Save to cache (2h TTL)
	if redispkg.Client != nil {
		if b, err := json.Marshal(result); err == nil {
			redispkg.Client.Set(redispkg.Ctx, cacheKey, b, 2*time.Hour)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result["data"],
		"meta":    result["meta"],
		"cached":  false,
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

	// Active Invalidation
	if redispkg.Client != nil {
		iter := redispkg.Client.Scan(redispkg.Ctx, 0, "admin:cinemas:list:*", 0).Iterator()
		var keysToDelete []string
		for iter.Next(redispkg.Ctx) {
			keysToDelete = append(keysToDelete, iter.Val())
		}
		if len(keysToDelete) > 0 {
			redispkg.Client.Del(redispkg.Ctx, keysToDelete...)
			log.Printf("[Cache INVALIDATED] %d keys cleared after UpdateCinema\n", len(keysToDelete))
		}
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

	// Active Invalidation
	if redispkg.Client != nil {
		iter := redispkg.Client.Scan(redispkg.Ctx, 0, "admin:cinemas:list:*", 0).Iterator()
		var keysToDelete []string
		for iter.Next(redispkg.Ctx) {
			keysToDelete = append(keysToDelete, iter.Val())
		}
		if len(keysToDelete) > 0 {
			redispkg.Client.Del(redispkg.Ctx, keysToDelete...)
			log.Printf("[Cache INVALIDATED] keys cleared after DeleteCinema id=%d\n", id)
		}
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

	// Active Invalidation
	if redispkg.Client != nil {
		iter := redispkg.Client.Scan(redispkg.Ctx, 0, "admin:cinemas:list:*", 0).Iterator()
		var keysToDelete []string
		for iter.Next(redispkg.Ctx) {
			keysToDelete = append(keysToDelete, iter.Val())
		}
		if len(keysToDelete) > 0 {
			redispkg.Client.Del(redispkg.Ctx, keysToDelete...)
			log.Printf("[Cache INVALIDATED] keys cleared after DeleteRoom id=%d\n", id)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Room deleted successfully"})
}

func (h *AdminHandler) UpdateRoom(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid room ID"})
		return
	}

	var req service.UpdateRoomReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	cinemaSvc := &service.CinemaService{}
	room, err := cinemaSvc.UpdateRoom(id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Real-time Update via SSE
	sse.BroadcastRoomUpdated(room.ID, room.CinemaID, room.Name, room.Capacity)

	c.JSON(http.StatusOK, gin.H{"success": true, "data": room})
}

func (h *AdminHandler) GenerateAILayout(c *gin.Context) {
	roomID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid room ID"})
		return
	}

	var req struct {
		Prompt string `json:"prompt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	cinemaSvc := &service.CinemaService{}
	seats, err := cinemaSvc.GenerateAILayout(roomID, req.Prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": seats})
}
