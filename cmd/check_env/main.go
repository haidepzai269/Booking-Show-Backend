package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	key := os.Getenv("GROQ_API_KEY")
	if key == "" {
		fmt.Println("GROQ_API_KEY is EMPTY in os.Getenv")
	} else {
		fmt.Printf("GROQ_API_KEY is set (len: %d), starts with %s\n", len(key), key[:5])
	}
}
