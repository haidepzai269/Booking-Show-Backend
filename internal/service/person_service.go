package service

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/go-resty/resty/v2"
)

type PersonService struct{}

type tmdbPersonResponse struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Biography    string `json:"biography"`
	Birthday     string `json:"birthday"`
	PlaceOfBirth string `json:"place_of_birth"`
	ProfilePath  string `json:"profile_path"`
	KnownFor     string `json:"known_for_department"`
}

func (s *PersonService) GetPersonDetail(tmdbID int) (*model.Person, error) {
	cacheKey := fmt.Sprintf("person:%d", tmdbID)

	// 1. Check Redis
	if redispkg.Client != nil {
		val, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result()
		if err == nil && val != "" {
			var person model.Person
			if err := json.Unmarshal([]byte(val), &person); err == nil && person.Biography != "" {
				return &person, nil
			}
		}
	}

	// 2. Check Database
	var person model.Person
	err := repository.DB.Where("tmdb_id = ?", tmdbID).First(&person).Error
	if err == nil && person.Biography != "" {
		// Cache and return
		s.cachePerson(&person)
		return &person, nil
	}

	// 3. Fetch from TMDB (hoặc Re-fetch nếu biography đang trống)
	log.Printf("Fetching person %d from TMDB (may including retry for bio)...", tmdbID)
	fetchedPerson, err := s.fetchFromTMDB(tmdbID)
	if err != nil {
		return nil, err
	}

	// 4. Save/Update to DB
	if person.ID != 0 {
		// Nếu đã có nhưng biography trống -> Update
		repository.DB.Model(&person).Updates(fetchedPerson)
	} else {
		if err := repository.DB.Create(fetchedPerson).Error; err != nil {
			log.Printf("Error saving person to DB: %v", err)
		}
	}

	// 5. Cache to Redis
	s.cachePerson(fetchedPerson)

	return fetchedPerson, nil
}

func (s *PersonService) fetchFromTMDB(tmdbID int) (*model.Person, error) {
	apiKey := os.Getenv("TMDB_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TMDB_API_KEY is not set")
	}

	client := resty.New()
	url := fmt.Sprintf("https://api.themoviedb.org/3/person/%d?api_key=%s&language=vi", tmdbID, apiKey)

	var res tmdbPersonResponse
	resp, err := client.R().SetResult(&res).Get(url)

	// Check if we need fallback to English: error, or successfully fetched but empty biography
	if err != nil || resp.IsError() || res.Biography == "" {
		log.Printf("Biography for %d in 'vi' is empty or error, trying 'en-US' and translate...", tmdbID)
		urlEn := fmt.Sprintf("https://api.themoviedb.org/3/person/%d?api_key=%s&language=en-US", tmdbID, apiKey)
		respEn, errEn := client.R().SetResult(&res).Get(urlEn)
		if errEn != nil || respEn.IsError() {
			if res.Name == "" { // Nếu cả tiếng Anh cũng không fetch được Name thì mới coi là lỗi
				return nil, fmt.Errorf("failed to fetch person from TMDB even in English: %v", errEn)
			}
		} else if res.Biography != "" {
			// Dịch sang tiếng Việt bằng LLM
			translatedBio := s.translateBiography(res.Biography)
			if translatedBio != "" {
				res.Biography = translatedBio
			}
		}
	}

	profileURL := ""
	if res.ProfilePath != "" {
		profileURL = "https://image.tmdb.org/t/p/w500" + res.ProfilePath
	}

	return &model.Person{
		TmdbID:       res.ID,
		Name:         res.Name,
		Biography:    res.Biography,
		Birthday:     res.Birthday,
		PlaceOfBirth: res.PlaceOfBirth,
		ProfilePath:  profileURL,
		KnownFor:     res.KnownFor,
	}, nil
}

func (s *PersonService) translateBiography(bio string) string {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		return ""
	}

	client := resty.New()
	prompt := fmt.Sprintf(`Dịch đoạn giới thiệu (biography) của diễn viên sau đây từ tiếng Anh sang tiếng Việt một cách tự nhiên, trang trọng. Chỉ trả về nội dung đã dịch, không thêm bất kỳ văn bản nào khác: %s`, bio)

	requestBody := map[string]interface{}{
		"model": "llama-3.1-8b-instant",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
	}

	var groqRes struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	resp, err := client.R().
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Content-Type", "application/json").
		SetBody(requestBody).
		SetResult(&groqRes).
		Post("https://api.groq.com/openai/v1/chat/completions")

	if err != nil || resp.IsError() || len(groqRes.Choices) == 0 {
		log.Printf("Groq translation error: %v", err)
		return ""
	}

	return groqRes.Choices[0].Message.Content
}

func (s *PersonService) cachePerson(person *model.Person) {
	if redispkg.Client == nil {
		return
	}
	cacheKey := fmt.Sprintf("person:%d", person.TmdbID)
	data, _ := json.Marshal(person)
	_ = redispkg.Client.Set(redispkg.Ctx, cacheKey, data, 30*24*time.Hour).Err()
}
