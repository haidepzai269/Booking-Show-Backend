package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
)

// haversine tính khoảng cách (km) giữa 2 tọa độ địa lý bằng công thức Haversine
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371 // Bán kính Trái Đất (km)
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return math.Round(R*c*100) / 100 // Làm tròn 2 chữ số thập phân
}

const cinemaCacheTTL = 24 * time.Hour

type CinemaService struct{}

func (s *CinemaService) ListCinemas() ([]model.Cinema, error) {
	const key = "cinemas:all"
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, key).Result(); err == nil {
			var cinemas []model.Cinema
			if json.Unmarshal([]byte(cached), &cinemas) == nil {
				log.Println("[Cache HIT] cinemas:all")
				return cinemas, nil
			}
		}
	}
	log.Println("[Cache MISS] cinemas:all - querying DB")

	var cinemas []model.Cinema
	if err := repository.DB.Where("is_active = ?", true).Find(&cinemas).Error; err != nil {
		return nil, err
	}

	if redispkg.Client != nil {
		if data, err := json.Marshal(cinemas); err == nil {
			redispkg.Client.Set(redispkg.Ctx, key, data, cinemaCacheTTL)
		}
	}
	return cinemas, nil
}

func (s *CinemaService) ListAdminCinemas(page, limit int, q string) ([]model.Cinema, int64, error) {
	var cinemas []model.Cinema
	var total int64

	query := repository.DB.Model(&model.Cinema{})

	if q != "" {
		query = query.Where("name ILIKE ?", "%"+q+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&cinemas).Error; err != nil {
		return nil, 0, err
	}

	return cinemas, total, nil
}

func (s *CinemaService) GetCinema(id int) (*model.Cinema, error) {
	key := fmt.Sprintf("cinemas:%d", id)
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, key).Result(); err == nil {
			var cinema model.Cinema
			if json.Unmarshal([]byte(cached), &cinema) == nil {
				log.Printf("[Cache HIT] %s\n", key)
				return &cinema, nil
			}
		}
	}

	var cinema model.Cinema
	if err := repository.DB.Where("id = ? AND is_active = ?", id, true).First(&cinema).Error; err != nil {
		return nil, errors.New("cinema not found")
	}

	if redispkg.Client != nil {
		if data, err := json.Marshal(cinema); err == nil {
			redispkg.Client.Set(redispkg.Ctx, key, data, cinemaCacheTTL)
		}
	}
	return &cinema, nil
}

type CreateCinemaServiceReq struct {
	Name      string
	Address   string
	City      string
	ImageURL  string
	Latitude  *float64
	Longitude *float64
}

func (s *CinemaService) CreateCinema(req CreateCinemaServiceReq) (*model.Cinema, error) {
	cinema := model.Cinema{
		Name:      req.Name,
		Address:   req.Address,
		City:      req.City,
		ImageURL:  req.ImageURL,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
	}

	if err := repository.DB.Create(&cinema).Error; err != nil {
		return nil, err
	}

	// Active Invalidation - xoa cache danh sach
	if redispkg.Client != nil {
		redispkg.Client.Del(redispkg.Ctx, "cinemas:all")
	}
	return &cinema, nil
}

type UpdateCinemaReq struct {
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	City      string   `json:"city"`
	ImageURL  string   `json:"image_url"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	IsActive  *bool    `json:"is_active"`
}

func (s *CinemaService) UpdateCinema(id int, req UpdateCinemaReq) (*model.Cinema, error) {
	var cinema model.Cinema
	if err := repository.DB.First(&cinema, id).Error; err != nil {
		return nil, errors.New("cinema not found")
	}

	if req.Name != "" {
		cinema.Name = req.Name
	}
	if req.Address != "" {
		cinema.Address = req.Address
	}
	if req.City != "" {
		cinema.City = req.City
	}
	if req.ImageURL != "" {
		cinema.ImageURL = req.ImageURL
	}
	if req.IsActive != nil {
		cinema.IsActive = *req.IsActive
	}
	// Cho phép cập nhật tọa độ (kể cả nil để xóa)
	if req.Latitude != nil {
		cinema.Latitude = req.Latitude
	}
	if req.Longitude != nil {
		cinema.Longitude = req.Longitude
	}

	if err := repository.DB.Save(&cinema).Error; err != nil {
		return nil, err
	}

	if redispkg.Client != nil {
		redispkg.Client.Del(redispkg.Ctx, "cinemas:all")
		redispkg.Client.Del(redispkg.Ctx, fmt.Sprintf("cinemas:%d", id))
	}

	return &cinema, nil
}

// cityCenterCoords - tọa độ trung tâm các thành phố lớn (fallback khi rạp chưa có tọa độ chính xác)
var cityCenterCoords = map[string][2]float64{
	"Hà Nội":      {21.0285, 105.8542},
	"Hồ Chí Minh": {10.7769, 106.7009},
	"Đà Nẵng":     {16.0544, 108.2022},
	"Cần Thơ":     {10.0452, 105.7469},
	"Hải Phòng":   {20.8449, 106.6881},
	"Bình Dương":  {11.1825, 106.6988},
	"Đồng Nai":    {10.9455, 106.8243},
	"Huế":         {16.4637, 107.5909},
	"Nha Trang":   {12.2388, 109.1967},
	"Vũng Tàu":    {10.4113, 107.1362},
}

// ListCinemasNearby - Lấy danh sách rạp, sắp xếp theo khoảng cách gần nhất nếu có lat/lng.
// Nếu rạp chưa có tọa độ chính xác, dùng tọa độ trung tâm thành phố làm fallback để sort.
func (s *CinemaService) ListCinemasNearby(userLat, userLng *float64) ([]model.CinemaWithDistance, error) {
	var cinemas []model.Cinema
	if err := repository.DB.Where("is_active = ?", true).Find(&cinemas).Error; err != nil {
		return nil, err
	}

	result := make([]model.CinemaWithDistance, 0, len(cinemas))
	for _, c := range cinemas {
		item := model.CinemaWithDistance{Cinema: c}

		if userLat != nil && userLng != nil {
			cinLat := c.Latitude
			cinLng := c.Longitude
			hasExactCoords := cinLat != nil && cinLng != nil

			// Fallback: nếu rạp chưa có tọa độ chính xác, dùng tọa độ trung tâm thành phố
			if !hasExactCoords && c.City != "" {
				if coords, ok := cityCenterCoords[c.City]; ok {
					lat, lng := coords[0], coords[1]
					cinLat = &lat
					cinLng = &lng
				}
			}

			if cinLat != nil && cinLng != nil {
				d := haversine(*userLat, *userLng, *cinLat, *cinLng)
				if hasExactCoords {
					// Tọa độ chính xác → trả về khoảng cách thực cho Frontend badge
					item.Distance = &d
				} else {
					// Fallback thành phố → chỉ dùng để sort, Frontend không hiển thị badge
					item.ApproxDistance = &d
				}
			}
		}

		result = append(result, item)
	}

	// Sort: gần nhất lên đầu (dùng Distance nếu có, fallback ApproxDistance)
	if userLat != nil && userLng != nil {
		sort.Slice(result, func(i, j int) bool {
			di := result[i].EffectiveDist()
			dj := result[j].EffectiveDist()
			if di == nil && dj == nil {
				return result[i].ID < result[j].ID
			}
			if di == nil {
				return false
			}
			if dj == nil {
				return true
			}
			return *di < *dj
		})
	}

	return result, nil
}

func (s *CinemaService) DeleteCinema(id int) error {
	var cinema model.Cinema
	if err := repository.DB.First(&cinema, id).Error; err != nil {
		return errors.New("cinema not found")
	}

	if err := repository.DB.Model(&cinema).Update("is_active", false).Error; err != nil {
		return err
	}

	if redispkg.Client != nil {
		redispkg.Client.Del(redispkg.Ctx, "cinemas:all")
		redispkg.Client.Del(redispkg.Ctx, fmt.Sprintf("cinemas:%d", id))
	}

	return nil
}

type RoomReq struct {
	Name     string `json:"name" binding:"required"`
	Capacity int    `json:"capacity" binding:"required,gt=0"`
}

func (s *CinemaService) CreateRoom(cinemaID int, req RoomReq) (*model.Room, error) {
	var dummy model.Cinema
	if err := repository.DB.First(&dummy, cinemaID).Error; err != nil {
		return nil, errors.New("cinema not found")
	}

	room := model.Room{
		CinemaID: cinemaID,
		Name:     req.Name,
		Capacity: req.Capacity,
	}

	tx := repository.DB.Begin()

	if err := tx.Create(&room).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Tự động sinh ghế STANDARD (mỗi hàng tối đa 16 ghế)
	cols := 16
	rows := (req.Capacity + cols - 1) / cols
	if rows > 26 {
		rows = 26 // Hỗ trợ tối đa từ A đến Z
	}

	var seats []model.Seat
	seatCount := 0

	for r := 0; r < rows; r++ {
		rowChar := string(rune('A' + r))
		for c := 1; c <= cols; c++ {
			if seatCount >= req.Capacity {
				break
			}
			seats = append(seats, model.Seat{
				RoomID:     room.ID,
				RowChar:    rowChar,
				SeatNumber: c,
				Type:       model.SeatStandard,
				IsActive:   true,
			})
			seatCount++
		}
	}

	if len(seats) > 0 {
		if err := tx.Create(&seats).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	// Active Invalidation - xoa cache phong cua rap nay
	if redispkg.Client != nil {
		roomKey := fmt.Sprintf("cinema:%d:rooms", cinemaID)
		redispkg.Client.Del(redispkg.Ctx, roomKey)
	}
	return &room, nil
}

func (s *CinemaService) DeleteRoom(id int) error {
	var room model.Room
	if err := repository.DB.First(&room, id).Error; err != nil {
		return errors.New("room not found")
	}

	if err := repository.DB.Model(&room).Update("is_active", false).Error; err != nil {
		return err
	}

	if redispkg.Client != nil {
		roomKey := fmt.Sprintf("cinema:%d:rooms", room.CinemaID)
		redispkg.Client.Del(redispkg.Ctx, roomKey)
	}

	return nil
}

// CinemaShowtimeItem — dung de tra ve API cinema movies
type CinemaShowtimeItem struct {
	ShowtimeID int       `json:"showtime_id"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	BasePrice  int       `json:"base_price"`
	RoomName   string    `json:"room_name"`
}

type CinemaMovieItem struct {
	MovieID         int                  `json:"movie_id"`
	Title           string               `json:"title"`
	PosterURL       string               `json:"poster_url"`
	DurationMinutes int                  `json:"duration_minutes"`
	Genres          []model.Genre        `json:"genres"`
	Showtimes       []CinemaShowtimeItem `json:"showtimes"`
}

// GetCinemaMovies - Lay danh sach phim dang chieu tai rap trong ngay (cache 5 phut)
func (s *CinemaService) GetCinemaMovies(cinemaID int, date time.Time) ([]CinemaMovieItem, error) {
	dateStr := date.Format("2006-01-02")
	key := fmt.Sprintf("cinema:%d:movies:%s", cinemaID, dateStr)

	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, key).Result(); err == nil {
			var result []CinemaMovieItem
			if json.Unmarshal([]byte(cached), &result) == nil {
				log.Printf("[Cache HIT] %s\n", key)
				return result, nil
			}
		}
	}
	log.Printf("[Cache MISS] %s - querying DB\n", key)

	// Lay toan bo phong cua rap
	var rooms []model.Room
	if err := repository.DB.Where("cinema_id = ? AND is_active = ?", cinemaID, true).Find(&rooms).Error; err != nil {
		return nil, err
	}
	if len(rooms) == 0 {
		return []CinemaMovieItem{}, nil
	}

	roomIDs := make([]int, len(rooms))
	roomNameMap := make(map[int]string)
	for i, r := range rooms {
		roomIDs[i] = r.ID
		roomNameMap[r.ID] = r.Name
	}

	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	var showtimes []model.Showtime
	if err := repository.DB.
		Preload("Movie").
		Preload("Movie.Genres").
		Where("room_id IN ? AND start_time >= ? AND start_time < ? AND is_active = ?",
			roomIDs, dayStart, dayEnd, true).
		Order("start_time ASC").
		Find(&showtimes).Error; err != nil {
		return nil, err
	}

	movieMap := make(map[int]*CinemaMovieItem)
	movieOrder := []int{}

	for _, st := range showtimes {
		movieID := st.Movie.ID
		if _, exists := movieMap[movieID]; !exists {
			movieMap[movieID] = &CinemaMovieItem{
				MovieID:         movieID,
				Title:           st.Movie.Title,
				PosterURL:       st.Movie.PosterURL,
				DurationMinutes: st.Movie.DurationMinutes,
				Genres:          st.Movie.Genres,
				Showtimes:       []CinemaShowtimeItem{},
			}
			movieOrder = append(movieOrder, movieID)
		}
		movieMap[movieID].Showtimes = append(movieMap[movieID].Showtimes, CinemaShowtimeItem{
			ShowtimeID: st.ID,
			StartTime:  st.StartTime,
			EndTime:    st.EndTime,
			BasePrice:  st.BasePrice,
			RoomName:   roomNameMap[st.RoomID],
		})
	}

	result := make([]CinemaMovieItem, 0, len(movieOrder))
	for _, id := range movieOrder {
		result = append(result, *movieMap[id])
	}

	// Cache 5 phut (lich chieu co the thay doi)
	if redispkg.Client != nil {
		if data, err := json.Marshal(result); err == nil {
			redispkg.Client.Set(redispkg.Ctx, key, data, 5*time.Minute)
		}
	}
	return result, nil
}

// GetCinemaRooms - Lay danh sach phong chieu cua rap (cache 24h)
func (s *CinemaService) GetCinemaRooms(cinemaID int) ([]model.Room, error) {
	key := fmt.Sprintf("cinema:%d:rooms", cinemaID)

	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, key).Result(); err == nil {
			var rooms []model.Room
			if json.Unmarshal([]byte(cached), &rooms) == nil {
				log.Printf("[Cache HIT] %s\n", key)
				return rooms, nil
			}
		}
	}

	var rooms []model.Room
	if err := repository.DB.
		Where("cinema_id = ? AND is_active = ?", cinemaID, true).
		Order("name ASC").
		Find(&rooms).Error; err != nil {
		return nil, err
	}

	if redispkg.Client != nil {
		if data, err := json.Marshal(rooms); err == nil {
			redispkg.Client.Set(redispkg.Ctx, key, data, cinemaCacheTTL)
		}
	}
	return rooms, nil
}
