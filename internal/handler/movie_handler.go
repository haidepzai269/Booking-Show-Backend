package handler

import (
	"net/http"
	"strconv"

	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/gin-gonic/gin"
)

type MovieHandler struct {
	MovieService      *service.MovieService
	MovieExtraService *service.MovieExtraService
}

func NewMovieHandler() *MovieHandler {
	return &MovieHandler{
		MovieService:      &service.MovieService{},
		MovieExtraService: &service.MovieExtraService{},
	}
}

func (h *MovieHandler) ListMovies(c *gin.Context) {
	movies, err := h.MovieService.ListMovies()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to fetch movies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": movies})
}

// SearchMovies - GET /movies/search?q=&genre_id=&sort=&limit=
func (h *MovieHandler) SearchMovies(c *gin.Context) {
	req := service.SearchMoviesReq{
		Query: c.Query("q"),
		Sort:  c.Query("sort"),
	}
	if gid, err := strconv.Atoi(c.Query("genre_id")); err == nil {
		req.GenreID = gid
	}
	if lim, err := strconv.Atoi(c.Query("limit")); err == nil {
		req.Limit = lim
	}

	movies, err := h.MovieService.SearchMovies(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to search movies"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": movies})
}

// GetHomeMovies - API đặc biệt cho trang chủ, trả về featured + hot + best-selling
func (h *MovieHandler) GetHomeMovies(c *gin.Context) {
	result, err := h.MovieService.GetHomeMovies()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to fetch home movies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *MovieHandler) GetMovie(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid movie ID"})
		return
	}

	movie, err := h.MovieService.GetMovie(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error(), "code": "NOT_FOUND"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": movie})
}

// Lấy thêm điểm số (TMDB), diễn viên (TMDB), và các câu đố trivia (Groq)
func (h *MovieHandler) GetMovieExtraInfo(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid movie ID"})
		return
	}

	result, err := h.MovieExtraService.GetExtraInfo(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

type GenreHandler struct {
	GenreService *service.GenreService
}

func NewGenreHandler() *GenreHandler {
	return &GenreHandler{
		GenreService: &service.GenreService{},
	}
}

func (h *GenreHandler) ListGenres(c *gin.Context) {
	genres, err := h.GenreService.ListGenres()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to fetch genres"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": genres})
}
