package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/pgvector/pgvector-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Dùng bản copy struct để tránh lỗi import internal
type Genre struct {
	ID   int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Name string `json:"name" gorm:"type:varchar(50);unique;not null"`
}

type Movie struct {
	ID              int              `json:"id" gorm:"primaryKey;autoIncrement"`
	Title           string           `json:"title" gorm:"type:varchar(255);not null"`
	Description     string           `json:"description" gorm:"type:text"`
	IsActive        bool             `json:"is_active" gorm:"default:true"`
	Embedding       *pgvector.Vector `json:"-" gorm:"type:vector(384)"`
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	hfToken := os.Getenv("HUGGINGFACE_TOKEN")
	hfURL := "https://router.huggingface.co/hf-inference/models/sentence-transformers/all-MiniLM-L6-v2/pipeline/feature-extraction"

	db, _ := gorm.Open(postgres.Open(dbURL), &gorm.Config{})

	var movies []Movie
	db.Where("is_active = ?", true).Find(&movies)

	fmt.Printf("Found %d active movies to check/update\n", len(movies))

	client := &http.Client{Timeout: 20 * time.Second}

	for _, m := range movies {
		fmt.Printf("Processing: %s... ", m.Title)
		
		// Luôn tạo mới embedding để chắc chắn
		text := m.Title + ". " + m.Description
		payload := map[string]interface{}{"inputs": text}
		jsonData, _ := json.Marshal(payload)

		req, _ := http.NewRequest("POST", hfURL, bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+hfToken)

		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != 200 {
			fmt.Printf("FAILED (Status: %d, Err: %v)\n", resp.StatusCode, err)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var vector []float32
		if err := json.Unmarshal(body, &vector); err != nil {
			fmt.Printf("PARSE ERROR: %v\n", err)
			continue
		}

		if len(vector) == 384 {
			v := pgvector.NewVector(vector)
			db.Model(&m).Update("embedding", v)
			fmt.Printf("SUCCESS\n")
		} else {
			fmt.Printf("WRONG DIMENSIONS: %d\n", len(vector))
		}
		
		// Tránh rate limit
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println("Done!")
}
