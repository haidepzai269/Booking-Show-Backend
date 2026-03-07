package handler

import (
	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/middleware"
	"github.com/gin-gonic/gin"
)

func SetupRouter(r *gin.Engine, cfg *config.Config) {
	v1 := r.Group("/api/v1")

	// ─── Handlers ────────────────────────────────────────────────────────────
	authHandler := NewAuthHandler(cfg)
	movieHandler := NewMovieHandler()
	genreHandler := NewGenreHandler()
	adminHandler := NewAdminHandler()
	adminSSEHandler := NewAdminSSEHandler()
	adminSearchHandler := NewAdminSearchHandler()
	cinemaHandler := NewCinemaHandler()
	concessionHandler := NewConcessionHandler()
	promotionHandler := NewPromotionHandler()
	showtimeHandler := NewShowtimeHandler()
	seatHandler := NewSeatHandler()
	orderHandler := NewOrderHandler()
	paymentHandler := NewPaymentHandler()
	ticketHandler := NewTicketHandler()
	personHandler := NewPersonHandler()
	faqHandler := NewFAQHandler()
	userHandler := NewUserHandler()
	campaignHandler := NewCampaignHandler()

	// ─── Public: Auth ────────────────────────────────────────────────────────
	auth := v1.Group("/auth")
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.RefreshToken)
		auth.POST("/magic-link", authHandler.RequestMagicLink)
		auth.POST("/magic-link/verify", authHandler.VerifyMagicLink)
		auth.POST("/reset-password", authHandler.ResetPassword)
	}

	// ─── Public: Movies ──────────────────────────────────────────────────────
	movies := v1.Group("/movies")
	{
		movies.GET("/", movieHandler.ListMovies)
		movies.GET("/home", movieHandler.GetHomeMovies)  // Redis cached: featured + hot + best-selling
		movies.GET("/search", movieHandler.SearchMovies) // Tìm kiếm nâng cao: ?q=&genre_id=&sort=
		movies.GET("/:id", movieHandler.GetMovie)
		movies.GET("/:id/extra", movieHandler.GetMovieExtraInfo)
		movies.GET("/:id/showtimes", showtimeHandler.GetShowtimes)
	}

	// ─── Public: Cinemas ─────────────────────────────────────────────────────
	cinemas := v1.Group("/cinemas")
	{
		cinemas.GET("/", cinemaHandler.ListCinemas)
		cinemas.GET("/:id", cinemaHandler.GetCinema)
		cinemas.GET("/:id/movies", cinemaHandler.GetCinemaMovies) // Phìm đang chiếu theo ngày
		cinemas.GET("/:id/rooms", cinemaHandler.GetCinemaRooms)   // Danh sách phòng chiếu
	}

	// ─── Public: Concessions (bắp nước) ─────────────────────────────────────
	concessions := v1.Group("/concessions")
	{
		concessions.GET("/", concessionHandler.ListConcessions)
		concessions.GET("/:id", concessionHandler.GetConcession)
	}

	// ─── Public: Genres ──────────────────────────────────────────────────────
	genres := v1.Group("/genres")
	{
		genres.GET("/", genreHandler.ListGenres)
	}

	// ─── Public: Persons ─────────────────────────────────────────────────────
	persons := v1.Group("/persons")
	{
		persons.GET("/:id", personHandler.GetPerson)
	}

	// ─── Public: FAQ (AI Chatbot) ────────────────────────────────────────────
	faq := v1.Group("/faq")
	{
		faq.POST("/ask", faqHandler.AskFAQ)
		faq.GET("/top", faqHandler.GetTopFAQs)
	}

	// ─── Public: Campaigns (Trang khuyến mãi) ───────────────────────────────────
	campaigns := v1.Group("/campaigns")
	{
		campaigns.GET("", campaignHandler.ListCampaigns)
		campaigns.GET("/:id", campaignHandler.GetCampaign)
	}

	// ─── Public: Showtimes ───────────────────────────────────────────────────
	showtimes := v1.Group("/showtimes")
	{
		showtimes.GET("/:id", showtimeHandler.GetShowtime)
		showtimes.GET("/:id/seats", seatHandler.GetSeats)
		showtimes.GET("/:id/seats/stream", seatHandler.GetSeatsStream)
	}

	// ─── Public: Payment callbacks (no auth — verified by gateway sig) ───────
	payments := v1.Group("/payments")
	{
		payments.GET("/vnpay_return", paymentHandler.VNPAYWebhook)
		payments.POST("/zalopay/callback", paymentHandler.ZaloPayCallback) // ZaloPay server→server
		payments.POST("/payos/callback", paymentHandler.PayOSWebhook)      // PayOS webhook
		payments.GET("/check_status", paymentHandler.CheckStatus)          // Frontend manual polling
	}

	// ─── Protected: Cần JWT token ────────────────────────────────────────────
	protected := v1.Group("/")
	protected.Use(middleware.AuthMiddleware(cfg))
	{
		// Auth
		protected.POST("/auth/logout", authHandler.Logout)

		// Seats
		protected.POST("/seats/lock", seatHandler.LockSeats)
		protected.DELETE("/seats/lock", seatHandler.UnlockSeats)
		protected.POST("/seats/unlock", seatHandler.UnlockSeats) // POST alias — hỗ trợ sendBeacon

		// Promotions (validate voucher — cần auth để tránh abuse)
		protected.POST("/promotions/validate", promotionHandler.Validate)

		// Orders
		protected.POST("/orders", orderHandler.CreateOrder)
		protected.GET("/orders/my", orderHandler.MyOrders)
		protected.GET("/orders/:id", orderHandler.GetOrder)
		protected.DELETE("/orders/:id", orderHandler.CancelOrder)
		protected.POST("/orders/:id/cancel", orderHandler.CancelOrder)                // POST alias — hỗ trợ sendBeacon
		protected.PUT("/orders/:id/concessions", orderHandler.UpdateOrderConcessions) // Cập nhật bắp nước
		protected.PUT("/orders/:id/voucher", orderHandler.ApplyOrderVoucher)          // Áp dụng/xóa voucher

		// Payments — tạo URL thanh toán
		protected.POST("/payments/initiate", paymentHandler.Initiate)

		// Tickets
		protected.GET("/tickets/my", ticketHandler.MyTickets)
		protected.GET("/tickets/:id", ticketHandler.GetTicket)

		// User profile
		protected.GET("/users/me", userHandler.GetMe)
		protected.PUT("/users/me", userHandler.UpdateMe)
		protected.PATCH("/users/theme", userHandler.UpdateTheme)
	}

	// ─── Staff only (Admin + Cinema Manager) ─────────────────────────────────
	staff := v1.Group("/")
	staff.Use(middleware.AuthMiddleware(cfg), middleware.RequireRole("ADMIN", "CINEMA_MANAGER"))
	{
		staff.POST("/tickets/:id/verify", ticketHandler.VerifyTicket)
	}

	// ─── Admin routes ────────────────────────────────────────────────────────
	admin := v1.Group("/admin")
	admin.Use(middleware.AuthMiddleware(cfg), middleware.RequireRole("ADMIN", "CINEMA_MANAGER"))
	{
		// Dashboard
		admin.GET("/stats", adminHandler.GetDashboardStats)

		// Movies CRUD
		admin.GET("/movies", adminHandler.ListAdminMovies)
		admin.POST("/movies", adminHandler.CreateMovie)
		admin.PUT("/movies/:id", adminHandler.UpdateMovie)
		admin.DELETE("/movies/:id", middleware.RequireRole("ADMIN"), adminHandler.DeleteMovie)

		// Cinemas & Rooms
		admin.GET("/cinemas", adminHandler.ListAdminCinemas)
		admin.POST("/cinemas", middleware.RequireRole("ADMIN"), adminHandler.CreateCinema)
		admin.PUT("/cinemas/:id", middleware.RequireRole("ADMIN"), adminHandler.UpdateCinema)
		admin.DELETE("/cinemas/:id", middleware.RequireRole("ADMIN"), adminHandler.DeleteCinema)
		admin.GET("/cinemas/:id/rooms", adminHandler.ListAdminRoomsByCinema)
		admin.POST("/cinemas/:id/rooms", adminHandler.CreateRoom)
		admin.DELETE("/rooms/:id", middleware.RequireRole("ADMIN"), adminHandler.DeleteRoom)
		admin.GET("/rooms/:id/seats", adminHandler.GetRoomSeats)
		admin.POST("/rooms/:id/seats", adminHandler.InitSeats)
		admin.PUT("/rooms/:id/seats/layout", adminHandler.UpdateSeatsLayout)

		// Concessions
		admin.GET("/concessions", adminHandler.ListAdminConcessions)
		admin.POST("/concessions", middleware.RequireRole("ADMIN"), adminHandler.CreateConcession)
		admin.PUT("/concessions/:id", middleware.RequireRole("ADMIN"), adminHandler.UpdateConcession)
		admin.DELETE("/concessions/:id", middleware.RequireRole("ADMIN"), adminHandler.DeleteConcession)

		// Promotions
		admin.GET("/promotions", adminHandler.ListAdminPromotions)
		admin.POST("/promotions", middleware.RequireRole("ADMIN"), adminHandler.CreatePromotion)
		admin.PUT("/promotions/:id", middleware.RequireRole("ADMIN"), adminHandler.UpdatePromotion)
		admin.DELETE("/promotions/:id", middleware.RequireRole("ADMIN"), adminHandler.DeletePromotion)

		// Showtimes CRUD
		admin.GET("/showtimes", adminHandler.ListAdminShowtimes)
		admin.POST("/showtimes", adminHandler.CreateShowtime)
		admin.PUT("/showtimes/:id", adminHandler.UpdateShowtime)
		admin.DELETE("/showtimes/:id", adminHandler.DeleteShowtime)

		// Orders, Users, Refunds
		admin.GET("/orders", adminHandler.ListAdminOrders)
		admin.GET("/orders/:id", adminHandler.GetOrderDetail)
		admin.GET("/users", middleware.RequireRole("ADMIN"), adminHandler.ListAdminUsers)
		admin.PUT("/users/:id/role", middleware.RequireRole("ADMIN"), adminHandler.UpdateUserRole)
		admin.GET("/refunds", middleware.RequireRole("ADMIN"), adminHandler.ListAdminRefunds)

		// Upload
		admin.POST("/upload", middleware.RequireRole("ADMIN"), adminHandler.Upload)

		// Search tổng hợp
		admin.GET("/search", adminSearchHandler.Search)

		// Campaigns (chiến dịch marketing)
		admin.GET("/campaigns", campaignHandler.AdminListCampaigns)
		admin.GET("/campaigns/:id", campaignHandler.AdminGetCampaign)
		admin.POST("/campaigns", middleware.RequireRole("ADMIN"), campaignHandler.AdminCreateCampaign)
		admin.PUT("/campaigns/:id", middleware.RequireRole("ADMIN"), campaignHandler.AdminUpdateCampaign)
		admin.DELETE("/campaigns/:id", middleware.RequireRole("ADMIN"), campaignHandler.AdminDeleteCampaign)

		// Notifications SSE stream
		admin.GET("/notifications/stream", adminSSEHandler.Stream)
	}
}
