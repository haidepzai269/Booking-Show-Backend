package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/booking-show/booking-show-api/pkg/cloudinary"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/gin-gonic/gin"
)

type AdminHandler struct{}

func NewAdminHandler() *AdminHandler {
	return &AdminHandler{}
}

// Upload - Upload file lên Cloudinary
func (h *AdminHandler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "file is required", "code": "INVALID_INPUT"})
		return
	}

	openedFile, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "unable to read file"})
		return
	}
	defer openedFile.Close()

	ctx := context.Background()
	uploadResult, err := cloudinary.CloudinaryClient.Upload.Upload(ctx, openedFile, uploader.UploadParams{
		Folder: "booking-show",
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "upload failed", "code": "UPLOAD_FAILED"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"url": uploadResult.SecureURL,
		},
	})
}

// GetDashboardStats - Thống kê tổng quan cho dashboard admin
func (h *AdminHandler) GetDashboardStats(c *gin.Context) {
	cacheKey := "admin:dashboard:stats"

	// 1. Try to get from Cache
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result(); err == nil {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(cached), &data); err == nil {
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"data":    data,
					"cached":  true,
				})
				return
			}
		}
	}

	// Tổng doanh thu (chỉ tính các đơn COMPLETED)
	var totalRevenue int64
	repository.DB.Model(&model.Order{}).
		Where("status = ?", model.OrderCompleted).
		Select("COALESCE(SUM(final_amount), 0)").
		Scan(&totalRevenue)

	// Tổng số đơn hàng
	var totalOrders int64
	repository.DB.Model(&model.Order{}).Count(&totalOrders)

	// Tổng số users
	var totalUsers int64
	repository.DB.Model(&model.User{}).Count(&totalUsers)

	// Tổng số vé đã bán
	var totalTickets int64
	repository.DB.Model(&model.Ticket{}).Count(&totalTickets)

	// Tổng số phim đang hoạt động
	var totalMovies int64
	repository.DB.Model(&model.Movie{}).Where("is_active = ?", true).Count(&totalMovies)

	// Doanh thu tháng này
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	var monthlyRevenue int64
	repository.DB.Model(&model.Order{}).
		Where("status = ? AND created_at >= ?", model.OrderCompleted, startOfMonth).
		Select("COALESCE(SUM(final_amount), 0)").
		Scan(&monthlyRevenue)

	// 10 đơn hàng gần nhất
	type RecentOrder struct {
		ID          string `json:"id"`
		UserName    string `json:"user_name"`
		UserEmail   string `json:"user_email"`
		MovieTitle  string `json:"movie_title"`
		FinalAmount int    `json:"final_amount"`
		Status      string `json:"status"`
		CreatedAt   string `json:"created_at"`
	}
	var recentOrders []RecentOrder
	repository.DB.Table("orders").
		Select("orders.id, users.full_name as user_name, users.email as user_email, movies.title as movie_title, orders.final_amount, orders.status, orders.created_at").
		Joins("JOIN users ON users.id = orders.user_id").
		Joins("JOIN showtimes ON showtimes.id = orders.showtime_id").
		Joins("JOIN movies ON movies.id = showtimes.movie_id").
		Order("orders.created_at DESC").
		Limit(10).
		Scan(&recentOrders)

	// Doanh thu 7 ngày gần đây (chart data)
	type DayRevenue struct {
		Date    string `json:"date"`
		Revenue int64  `json:"revenue"`
		Orders  int64  `json:"orders"`
	}
	var chartData []DayRevenue
	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		startOfDay := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
		endOfDay := startOfDay.Add(24 * time.Hour)

		var dayRev int64
		repository.DB.Model(&model.Order{}).
			Where("status = ? AND created_at >= ? AND created_at < ?", model.OrderCompleted, startOfDay, endOfDay).
			Select("COALESCE(SUM(final_amount), 0)").
			Scan(&dayRev)

		var dayOrders int64
		repository.DB.Model(&model.Order{}).
			Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay).
			Count(&dayOrders)

		chartData = append(chartData, DayRevenue{
			Date:    day.Format("2006-01-02"),
			Revenue: dayRev,
			Orders:  dayOrders,
		})
	}

	data := gin.H{
		"total_revenue":   totalRevenue,
		"total_orders":    totalOrders,
		"total_users":     totalUsers,
		"total_tickets":   totalTickets,
		"total_movies":    totalMovies,
		"monthly_revenue": monthlyRevenue,
		"recent_orders":   recentOrders,
		"chart_data":      chartData,
	}

	// Save to cache
	if redispkg.Client != nil {
		if b, err := json.Marshal(data); err == nil {
			redispkg.Client.Set(redispkg.Ctx, cacheKey, b, 5*time.Minute)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
		"cached":  false,
	})
}

// ListAdminMovies - Danh sách phim (có pagination + search + filter)
func (h *AdminHandler) ListAdminMovies(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	q := c.Query("q")
	onlyActive := c.Query("active") == "true"

	cacheKey := "admin:movies:list:" + strconv.Itoa(page) + ":" + strconv.Itoa(limit) + ":" + q + ":" + strconv.FormatBool(onlyActive)

	// Get from Cache
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result(); err == nil {
			var cachedResult map[string]interface{}
			if err := json.Unmarshal([]byte(cached), &cachedResult); err == nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": cachedResult, "cached": true})
				return
			}
		}
	}

	movieService := &service.MovieService{}
	result, err := movieService.ListAdminMovies(page, limit, q, onlyActive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Save to cache (2h TTL)
	if redispkg.Client != nil {
		if b, err := json.Marshal(result); err == nil {
			redispkg.Client.Set(redispkg.Ctx, cacheKey, b, 2*time.Hour)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result, "cached": false})
}

// CreateMovie - Tạo phim mới
func (h *AdminHandler) CreateMovie(c *gin.Context) {
	var req service.CreateMovieReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error(), "code": "INVALID_INPUT"})
		return
	}

	movieService := &service.MovieService{}
	movie, err := movieService.CreateMovie(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Active Invalidation (Movies & Dashboard)
	if redispkg.Client != nil {
		// Clear search + map admin cache
		iter := redispkg.Client.Scan(redispkg.Ctx, 0, "movies:search:*", 0).Iterator()
		var keysToDelete []string
		for iter.Next(redispkg.Ctx) {
			keysToDelete = append(keysToDelete, iter.Val())
		}
		adminIter := redispkg.Client.Scan(redispkg.Ctx, 0, "admin:movies:list:*", 0).Iterator()
		for adminIter.Next(redispkg.Ctx) {
			keysToDelete = append(keysToDelete, adminIter.Val())
		}
		keysToDelete = append(keysToDelete, "admin:dashboard:stats") // Invalidate dashboard stats

		if len(keysToDelete) > 0 {
			redispkg.Client.Del(redispkg.Ctx, keysToDelete...)
			log.Printf("[Cache INVALIDATED] %d keys cleared after CreateMovie\n", len(keysToDelete))
		}
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": movie})
}

// UpdateMovie - Cập nhật thông tin phim
func (h *AdminHandler) UpdateMovie(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid movie ID"})
		return
	}

	var req service.UpdateMovieReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error(), "code": "INVALID_INPUT"})
		return
	}

	movieService := &service.MovieService{}
	movie, err := movieService.UpdateMovie(id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Active Invalidation
	if redispkg.Client != nil {
		iter := redispkg.Client.Scan(redispkg.Ctx, 0, "movies:search:*", 0).Iterator()
		var keysToDelete []string
		for iter.Next(redispkg.Ctx) {
			keysToDelete = append(keysToDelete, iter.Val())
		}
		adminIter := redispkg.Client.Scan(redispkg.Ctx, 0, "admin:movies:list:*", 0).Iterator()
		for adminIter.Next(redispkg.Ctx) {
			keysToDelete = append(keysToDelete, adminIter.Val())
		}

		if len(keysToDelete) > 0 {
			redispkg.Client.Del(redispkg.Ctx, keysToDelete...)
			log.Printf("[Cache INVALIDATED] %d keys cleared after UpdateMovie\n", len(keysToDelete))
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": movie})
}

// DeleteMovie - Soft delete phim
func (h *AdminHandler) DeleteMovie(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid movie ID"})
		return
	}

	movieService := &service.MovieService{}
	if err := movieService.DeleteMovie(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Active Invalidation
	if redispkg.Client != nil {
		iter := redispkg.Client.Scan(redispkg.Ctx, 0, "movies:search:*", 0).Iterator()
		var keysToDelete []string
		for iter.Next(redispkg.Ctx) {
			keysToDelete = append(keysToDelete, iter.Val())
		}
		adminIter := redispkg.Client.Scan(redispkg.Ctx, 0, "admin:movies:list:*", 0).Iterator()
		for adminIter.Next(redispkg.Ctx) {
			keysToDelete = append(keysToDelete, adminIter.Val())
		}
		keysToDelete = append(keysToDelete, "admin:dashboard:stats")

		if len(keysToDelete) > 0 {
			redispkg.Client.Del(redispkg.Ctx, keysToDelete...)
			log.Printf("[Cache INVALIDATED] search cache cleared after DeleteMovie id=%d\n", id)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": "Movie deleted successfully"})
}

// UpdateShowtime - Cập nhật suất chiếu
func (h *AdminHandler) UpdateShowtime(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid showtime ID"})
		return
	}

	var req service.UpdateShowtimeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	showtimeService := &service.ShowtimeService{}
	showtime, err := showtimeService.UpdateShowtime(id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": showtime})
}

// DeleteShowtime - Xóa suất chiếu (soft delete)
func (h *AdminHandler) DeleteShowtime(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid showtime ID"})
		return
	}

	showtimeService := &service.ShowtimeService{}
	if err := showtimeService.DeleteShowtime(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": "Showtime deleted successfully"})
}

// ListAdminShowtimes - Danh sách suất chiếu cho admin
func (h *AdminHandler) ListAdminShowtimes(c *gin.Context) {
	movieID, _ := strconv.Atoi(c.Query("movie_id"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	showtimeService := &service.ShowtimeService{}
	showtimes, total, err := showtimeService.ListAdminShowtimes(movieID, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"showtimes": showtimes,
			"total":     total,
			"page":      page,
			"limit":     limit,
		},
	})
}

// ─── MODULE 4: ORDERS, USERS, REFUNDS ──────────────────────────────────────

func (h *AdminHandler) ListAdminOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "15"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 15
	}
	q := c.Query("q")

	cacheKey := "admin:orders:list:" + strconv.Itoa(page) + ":" + strconv.Itoa(limit) + ":" + q

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

	orderSvc := &service.OrderService{}
	orders, total, err := orderSvc.ListAdminOrders(page, limit, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	result := gin.H{
		"data": orders,
		"meta": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	}

	// Save to cache (30s TTL cho Orders vì tính real-time cao)
	if redispkg.Client != nil {
		if b, err := json.Marshal(result); err == nil {
			redispkg.Client.Set(redispkg.Ctx, cacheKey, b, 30*time.Second)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result["data"],
		"meta":    result["meta"],
		"cached":  false,
	})
}

func (h *AdminHandler) GetOrderDetail(c *gin.Context) {
	orderIDStr := c.Param("id")

	orderSvc := &service.OrderService{}
	order, err := orderSvc.GetOrder(orderIDStr, 0, true) // isAdmin = true
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}

	order.User.PasswordHash = ""

	c.JSON(http.StatusOK, gin.H{"success": true, "data": order})
}

func (h *AdminHandler) ListAdminUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	q := c.Query("q")

	userSvc := &service.UserService{}
	users, total, err := userSvc.ListAdminUsers(page, limit, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    users,
		"meta": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

type UpdateUserRoleReq struct {
	Role string `json:"role" binding:"required"`
}

func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid user ID"})
		return
	}

	var req UpdateUserRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	userSvc := &service.UserService{}
	if err := userSvc.UpdateUserRole(id, req.Role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Role updated successfully"})
}

func (h *AdminHandler) ListAdminRefunds(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	orderSvc := &service.OrderService{}
	orders, total, err := orderSvc.ListAdminRefunds(page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    orders,
		"meta": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}
