package service

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
)

// ToolDefinition định nghĩa cấu trúc tool gửi lên AI
type ToolDefinition struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ToolCall đại diện cho yêu cầu gọi tool từ AI
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// GetAvailableTools trả về danh sách các công cụ AI có thể sử dụng
func GetAvailableTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_showtimes",
				Description: "Lấy danh sách suất chiếu của một bộ phim. Trả về thông tin rạp, phòng và thời gian chiếu.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"movie_id": map[string]interface{}{
							"type":        "integer",
							"description": "ID của phim cần tra cứu suất chiếu",
						},
						"movie_title": map[string]interface{}{
							"type":        "string",
							"description": "Tên phim nếu không có ID (dùng để tìm kiếm ID trước)",
						},
					},
					"required": []string{"movie_title"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "search_movies",
				Description: "Tìm kiếm phim theo tên, thể loại hoặc mô tả.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Từ khóa tìm kiếm phim",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_seat_map",
				Description: "Lấy sơ đồ ghế ngồi và trạng thái trống của một suất chiếu cụ thể.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"showtime_id": map[string]interface{}{
							"type":        "integer",
							"description": "ID của suất chiếu cần xem sơ đồ ghế",
						},
					},
					"required": []string{"showtime_id"},
				},
			},
		},
	}
}

// ExecuteTool thực thi hàm dựa trên yêu cầu từ AI
func ExecuteTool(name string, args string) (string, error) {
	log.Printf("[AI Tool] Executing: %s with args: %s\n", name, args)
	
	switch name {
	case "get_showtimes":
		var argData struct {
			MovieID    int    `json:"movie_id"`
			MovieTitle string `json:"movie_title"`
		}
		if err := json.Unmarshal([]byte(args), &argData); err != nil {
			return "", err
		}

		var movieID = argData.MovieID
		// Nếu AI chỉ đưa Title, ta thực hiện tìm ID từ DB
		if movieID == 0 && argData.MovieTitle != "" {
			log.Printf("[AI Tool] Searching ID for title: %s\n", argData.MovieTitle)
			var movie model.Movie
			if err := repository.DB.Where("title ILIKE ? AND is_active = ?", "%"+argData.MovieTitle+"%", true).First(&movie).Error; err == nil {
				movieID = int(movie.ID)
				log.Printf("[AI Tool] Found Movie ID: %d for Title: %s\n", movieID, movie.Title)
			}
		}

		if movieID == 0 {
			return "Không tìm thấy bộ phim nào có tên tương tự trong hệ thống. Vui lòng kiểm tra lại tên phim.", nil
		}

		showtimeSvc := &ShowtimeService{}
		showtimes, err := showtimeSvc.GetShowtimesByMovie(movieID)
		if err != nil {
			return "", err
		}

		if len(showtimes) == 0 {
			return "Hiện tại phim này chưa có suất chiếu phù hợp, bạn vui lòng chọn phim khác hoặc quay lại sau nhé!", nil
		}

		res, _ := json.Marshal(showtimes)
		return string(res), nil

	case "search_movies":
		var argData struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(args), &argData); err != nil {
			return "", err
		}

		movieSvc := &MovieService{}
		movies, err := movieSvc.SearchMovies(SearchMoviesReq{
			Query: argData.Query,
			Limit: 5,
		})
		if err != nil {
			return "", err
		}

		if len(movies) == 0 {
			return "Không tìm thấy phim nào phù hợp với yêu cầu của bạn.", nil
		}

		res, _ := json.Marshal(movies)
		return string(res), nil

	case "get_seat_map":
		var argData struct {
			ShowtimeID int `json:"showtime_id"`
		}
		if err := json.Unmarshal([]byte(args), &argData); err != nil {
			return "", err
		}

		seatSvc := &SeatService{}
		seats, err := seatSvc.GetSeats(argData.ShowtimeID)
		if err != nil {
			return "", err
		}

		if len(seats) == 0 {
			return "Không tìm thấy dữ liệu ghế ngồi cho suất chiếu này.", nil
		}

		totalSeats := len(seats)
		availableSeats := 0
		var availableList []string

		for _, s := range seats {
			if s.Status == "AVAILABLE" {
				availableSeats++
				// Giả sử Seat có RowChar và SeatNumber (cần check model ShowtimeSeat)
				// Preload("Seat") đã được thực hiện trong GetSeats
				label := fmt.Sprintf("%s%d", s.Seat.RowChar, s.Seat.SeatNumber)
				availableList = append(availableList, label)
			}
		}

		result := map[string]interface{}{
			"showtime_id":     argData.ShowtimeID,
			"total_seats":     totalSeats,
			"available_seats": availableSeats,
			"empty_seats":     availableList,
		}

		res, _ := json.Marshal(result)
		return string(res), nil
	}

	return "", fmt.Errorf("tool not found: %s", name)
}
