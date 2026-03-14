package service

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/go-resty/resty/v2"
)

type MovieExtraService struct{}

type CastMember struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Character    string `json:"character"`
	ProfileImage string `json:"profile_image"`
}

type CrewMember struct {
	Name string `json:"name"`
	Job  string `json:"job"`
}

type MovieTrivia struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type MovieExtraData struct {
	Rating   float64       `json:"rating"`
	Cast     []CastMember  `json:"cast"`
	Director string        `json:"director"` // Đạo diễn (Từ Crew)
	Trivias  []MovieTrivia `json:"trivias"`
}

// Groq API Response structures
type groqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// TMDB API Response structures
type tmdbSearchResponse struct {
	Results []struct {
		ID          int     `json:"id"`
		VoteAverage float64 `json:"vote_average"`
	} `json:"results"`
}

type tmdbCreditsResponse struct {
	Cast []struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Character   string `json:"character"`
		ProfilePath string `json:"profile_path"`
	} `json:"cast"`
	Crew []CrewMember `json:"crew"`
}

// GetExtraInfo fetches from Redis cache or combined TMDB/Groq APIs
func (s *MovieExtraService) GetExtraInfo(movieID int) (*MovieExtraData, error) {
	cacheKey := fmt.Sprintf("movie_extra:%d", movieID)
	// Try getting from cache
	if redispkg.Client != nil {
		val, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result()
		if err == nil && val != "" {
			var cachedData MovieExtraData
			if json.Unmarshal([]byte(val), &cachedData) == nil {
				return &cachedData, nil
			}
		}
	}

	// Fetch Movie string from DB to search
	var movie model.Movie
	if err := repository.DB.First(&movie, movieID).Error; err != nil {
		return nil, fmt.Errorf("movie not found")
	}

	result := &MovieExtraData{}
	var wg sync.WaitGroup
	wg.Add(2)

	// Fetch TMDB (Rating & Cast & Director)
	go func() {
		defer wg.Done()
		rating, cast, director := fetchTMDBData(movie.Title)
		result.Rating = rating
		result.Cast = cast
		result.Director = director
	}()

	// Fetch Groq (Trivias)
	go func() {
		defer wg.Done()
		trivias := fetchGroqTrivias(movie.Title)
		result.Trivias = trivias
	}()

	wg.Wait()

	var hasFallback bool
	// Ensure fallback trivias if Groq fails
	if len(result.Trivias) == 0 {
		hasFallback = true
		result.Trivias = []MovieTrivia{
			{
				Question: "Bạn có biết?",
				Answer:   fmt.Sprintf("Bộ phim '%s' hiện đang là một trong những phim hot nhất tại BookingShow.", movie.Title),
			},
		}
	}

	// Cache the result to Redis
	if redispkg.Client != nil {
		if data, err := json.Marshal(result); err == nil {
			ttl := 7 * 24 * time.Hour
			if hasFallback {
				ttl = 1 * time.Minute // Nếu LLM xịt, chỉ cache 1 phút để người sau f5 lại
			}
			_ = redispkg.Client.Set(redispkg.Ctx, cacheKey, data, ttl).Err()
		}
	}

	return result, nil
}

func fetchTMDBData(movieTitle string) (float64, []CastMember, string) {
	apiKey := os.Getenv("TMDB_API_KEY")
	if apiKey == "" {
		log.Println("TMDB_API_KEY is not set")
		return 8.5, getDefaultCast(), "Đang cập nhật"
	}

	client := resty.New()

	// Search Movie
	searchURL := fmt.Sprintf("https://api.themoviedb.org/3/search/movie?api_key=%s&query=%s&language=vi", apiKey, url.QueryEscape(movieTitle))
	var searchRes tmdbSearchResponse
	resp, err := client.R().SetResult(&searchRes).Get(searchURL)

	if err != nil || resp.IsError() || len(searchRes.Results) == 0 {
		return 8.5, getDefaultCast(), "Chưa xác định"
	}

	tmdbMovieID := searchRes.Results[0].ID
	rating := searchRes.Results[0].VoteAverage

	// Get Credits
	creditsURL := fmt.Sprintf("https://api.themoviedb.org/3/movie/%d/credits?api_key=%s&language=vi", tmdbMovieID, apiKey)
	var creditsRes tmdbCreditsResponse
	resp, err = client.R().SetResult(&creditsRes).Get(creditsURL)

	if err != nil || resp.IsError() || len(creditsRes.Cast) == 0 {
		return rating, getDefaultCast(), "Đang cập nhật"
	}

	// Director
	director := "Đang cập nhật"
	for _, crew := range creditsRes.Crew {
		if crew.Job == "Director" {
			director = crew.Name
			break
		}
	}

	var cast []CastMember
	for i, c := range creditsRes.Cast {
		if i >= 6 { // Lấy top 6 diễn viên
			break
		}
		img := ""
		if c.ProfilePath != "" {
			img = "https://image.tmdb.org/t/p/w500" + c.ProfilePath
		} else {
			img = "https://images.unsplash.com/photo-1544005313-94ddf0286df2?q=80&w=250&auto=format&fit=crop"
		}
		cast = append(cast, CastMember{
			ID:           c.ID,
			Name:         c.Name,
			Character:    c.Character,
			ProfileImage: img,
		})
	}
	return rating, cast, director
}

func fetchGroqTrivias(movieTitle string) []MovieTrivia {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		log.Println("GROQ_API_KEY is not set")
		return []MovieTrivia{}
	}

	client := resty.New()
	prompt := fmt.Sprintf(`Đóng vai một chuyên gia điện ảnh, hãy thông báo 3 câu hỏi (trivia) thú vị và sự thật ít người biết về bộ phim '%s'. 

Yêu cầu định dạng JSON:
- Trả về một mảng JSON (Array), mỗi phần tử có "question" và "answer".
- Ví dụ mẫu: [{"question": "...", "answer": "..."}, {"question": "...", "answer": "..."}]
- CHỈ trả về JSON nguyên chất, KHÔNG giải thích, KHÔNG chứa markdown block.`, movieTitle)

	requestBody := map[string]interface{}{
		"model": "llama-3.1-8b-instant",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.3, // Giảm temperature để output ổn định hơn
	}

	var groqRes groqResponse
	resp, err := client.R().
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Content-Type", "application/json").
		SetBody(requestBody).
		SetResult(&groqRes).
		Post("https://api.groq.com/openai/v1/chat/completions")

	if err != nil || resp.IsError() || len(groqRes.Choices) == 0 {
		log.Println("Groq Error:", resp.String())
		return []MovieTrivia{}
	}

	content := groqRes.Choices[0].Message.Content

	// Thử bóc tách JSON từ chuỗi (đề phòng AI có thêm text)
	jsonPattern := regexp.MustCompile(`(?s)\[.*\]`)
	match := jsonPattern.FindString(content)
	if match != "" {
		content = match
	}

	var trivias []MovieTrivia
	if err := json.Unmarshal([]byte(content), &trivias); err != nil {
		// FALLBACK: Nếu AI trả về dạng Object { "Cau 1": {...} }, ta thử parse sang map rồi lấy value
		var mapFallback map[string]MovieTrivia
		objPattern := regexp.MustCompile(`(?s)\{.*\}`)
		objMatch := objPattern.FindString(content)
		if objMatch != "" {
			if errMap := json.Unmarshal([]byte(objMatch), &mapFallback); errMap == nil {
				for _, v := range mapFallback {
					trivias = append(trivias, v)
				}
				if len(trivias) > 0 {
					return trivias
				}
			}
		}

		log.Println("Groq JSON parse error:", err, "Content:", content)
		return []MovieTrivia{}
	}

	return trivias
}

func getDefaultCast() []CastMember {
	return []CastMember{
		{Name: "Đang cập nhật", Character: "Nhân vật chính", ProfileImage: "https://images.unsplash.com/photo-1506794778202-cad84cf45f1d?q=80&w=250&auto=format&fit=crop"},
		{Name: "Đang cập nhật", Character: "Nhân vật phụ", ProfileImage: "https://images.unsplash.com/photo-1534528741775-53994a69daeb?q=80&w=250&auto=format&fit=crop"},
	}
}
