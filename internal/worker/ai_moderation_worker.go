package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
)

type AIModerationJob struct {
	ReviewID int    `json:"review_id"`
	Content  string `json:"content"`
	UserID   int    `json:"user_id"`
}

func StartAIModerationWorker() {
	var lastCall time.Time

	go func() {
		for {
			if redispkg.Client == nil {
				time.Sleep(1 * time.Second)
				continue
			}

			// BRPop: Chờ lấy lấy 1 bình luận (Block tối đa 2 giây)
			result, err := redispkg.Client.BRPop(redispkg.Ctx, 2*time.Second, "queue:ai_moderation").Result()
			if err != nil {
				// Queue trống, lặp lại
				continue
			}

			// Đã tóm được 1 bình luận (BRPop trả về mảng 2 phần tử [key, value])
			rawJobs := []string{result[1]}
			
			// Quét nhanh xem còn ai kẹt trong hàng đợi không, vơ vét thêm tối đa 31 cái nữa
			for i := 0; i < 31; i++ {
				res, err := redispkg.Client.RPop(redispkg.Ctx, "queue:ai_moderation").Result()
				if err != nil {
					break
				}
				rawJobs = append(rawJobs, res)
			}

			// Tính toán khoảng ngủ: Gemini cho 15 Request / Phút ở Free Tier -> Tối đa 1 Request mỗi 4 giây
			elapsed := time.Since(lastCall)
			if elapsed < 4*time.Second {
				sleepDuration := (4 * time.Second) - elapsed
				time.Sleep(sleepDuration)
			}

			var jobs []AIModerationJob
			for _, r := range rawJobs {
				var job AIModerationJob
				if err := json.Unmarshal([]byte(r), &job); err == nil {
					jobs = append(jobs, job)
				}
			}

			if len(jobs) == 0 {
				continue
			}

			log.Printf("[AI Moderation Worker] 🚀 Tốc biến gửi BATCH %d bình luận cho Gemini AI...", len(jobs))
			lastCall = time.Now()

			var inputs []string
			for _, j := range jobs {
				inputs = append(inputs, j.Content)
			}

			scores, err := analyzeToxicityBatch(inputs)
			if err != nil && err.Error() == "429" {
				log.Println("[AI Moderation Worker] ⚠️ Nhận 429 từ Gemini (Rate Limit). Trả toàn bộ Batch về Queue...")
				for _, raw := range rawJobs {
					redispkg.Client.RPush(redispkg.Ctx, "queue:ai_moderation", raw)
				}
				lastCall = time.Now().Add(4 * time.Second) // Phạt thêm 4s
				continue
			}

			if err != nil {
				scores = make([]float64, len(jobs))
			}

			for i, job := range jobs {
				score := 0.0
				if i < len(scores) {
					score = scores[i]
				}
				processModerationResult(job, score)
			}
		}
	}()
	log.Println("[AI Moderation Worker] Started SMART BATCH processing (Instant if idle, Batched if overloaded) via Gemini 1.5")
}

func processModerationResult(job AIModerationJob, score float64) {
	log.Printf("[AI Moderation Worker] ReviewID %d -> Toxic score: %f\n", job.ReviewID, score)

	var review model.Review
	if err := repository.DB.First(&review, job.ReviewID).Error; err != nil {
		return
	}

	review.ToxicScore = score
	strikeCount := 0
	if score >= 0.8 {
		review.Status = model.StatusHidden
		
		var user model.User
		if err := repository.DB.First(&user, job.UserID).Error; err == nil {
			user.StrikeCount += 1
			strikeCount = user.StrikeCount
			if user.StrikeCount >= 3 {
				muteTime := time.Now().Add(3 * 24 * time.Hour)
				user.MutedUntil = &muteTime
			}
			repository.DB.Save(&user)
		}
	}
	repository.DB.Save(&review)

	keysPattern := fmt.Sprintf("movies:%d:reviews:*", review.MovieID)
	iter := redispkg.Client.Scan(redispkg.Ctx, 0, keysPattern, 0).Iterator()
	var keys []string
	for iter.Next(redispkg.Ctx) {
		keys = append(keys, iter.Val())
	}
	if len(keys) > 0 {
		redispkg.Client.Del(redispkg.Ctx, keys...)
	}

	channel := fmt.Sprintf("movie:%d:reviews", review.MovieID)
	if score < 0.8 {
		var fullReview model.Review
		if err := repository.DB.Preload("User").First(&fullReview, job.ReviewID).Error; err == nil {
			payload, _ := json.Marshal(map[string]interface{}{
				"action": "CREATED",
				"data":   fullReview,
			})
			redispkg.Client.Publish(redispkg.Ctx, channel, string(payload))
		}
	} else {
		payload, _ := json.Marshal(map[string]interface{}{
			"action":       "REJECTED",
			"review_id":    job.ReviewID,
			"user_id":      job.UserID,
			"strike_count": strikeCount,
		})
		redispkg.Client.Publish(redispkg.Ctx, channel, string(payload))
	}
}

func analyzeToxicityBatch(texts []string) ([]float64, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Println("[AI Moderation Worker] GEMINI_API_KEY is not set, skipping API call")
		return nil, nil
	}

	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=" + apiKey

	// Ép Output JSON thông minh bằng Prompt
	prompt := "Vào vai hệ thống kiểm duyệt ngôn từ tiếng Việt. Hãy chấm điểm mức độ 'độc hại' (tục tĩu, chửi thề, thù ghét, văng tục, xúc phạm) từ 0.0 (an toàn tuyệt đối) đến 1.0 (như cc, vcl, đm, tục tĩu nặng) cho danh sách bình luận sau. BẮT BUỘC chỉ trả về duy nhất 1 mảng JSON chứa các số Float tương ứng theo đúng thứ tự mảng đầu vào, tuyệt đối KHÔNG chứa bất kỳ câu chữ, markdown hay ký tự nào ngoài mảng JSON (ví dụ: [0.0, 0.9, 0.1]).\n\nDanh sách bình luận:\n"
	
	bytesData, _ := json.Marshal(texts)
	prompt += string(bytesData)

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"safetySettings": []map[string]interface{}{
			{"category": "HARM_CATEGORY_HARASSMENT", "threshold": "BLOCK_NONE"},
			{"category": "HARM_CATEGORY_HATE_SPEECH", "threshold": "BLOCK_NONE"},
			{"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT", "threshold": "BLOCK_NONE"},
			{"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "BLOCK_NONE"},
		},
		"generationConfig": map[string]interface{}{
			"responseMimeType": "application/json",
		},
	}

	jsonValue, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonValue))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("429")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[AI Moderation Worker] Gemini API Non-OK HTTP status: %d, response: %s\n", resp.StatusCode, string(body))
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(body, &geminiResp); err != nil {
		log.Printf("[AI Moderation Worker] Failed to parse Gemini response: %v\n", err)
		return nil, err
	}

	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates returned")
	}

	candidate := geminiResp.Candidates[0]
	if candidate.FinishReason == "SAFETY" || candidate.FinishReason == "RECITATION" {
		log.Printf("[AI Moderation Worker] Gemini đã chặn prompt do SAFETY FILTER. Phạt toàn bộ lô này thành 1.0")
		// Đánh dấu độc hại toàn bộ lô bị filter chặn
		scores := make([]float64, len(texts))
		for i := range scores {
			scores[i] = 1.0
		}
		return scores, nil
	}

	if len(candidate.Content.Parts) == 0 {
		return nil, fmt.Errorf("no content parts")
	}

	textResp := candidate.Content.Parts[0].Text
	
	var scores []float64
	if err := json.Unmarshal([]byte(textResp), &scores); err != nil {
		log.Printf("[AI Moderation Worker] Lỗi Parse JSON mảng float từ Gemini: %v. Text gốc: %s\n", err, textResp)
		return nil, err
	}

	// Đảm bảo rổ score trả ra luôn phải khớp hoặc dài hơn đầu vào để vòng for ngoài worker ko Panic
	if len(scores) < len(texts) {
		for i := len(scores); i < len(texts); i++ {
			scores = append(scores, 0.0) // Chỗ thiếu mặc định 0.0
		}
	}

	return scores, nil
}
