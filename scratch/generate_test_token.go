package main

import (
	"fmt"
	"time"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"os"
)

func main() {
	_ = godotenv.Load(".env")
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		fmt.Println("JWT_SECRET not found in .env")
		return
	}

	jti := uuid.New().String()
	claims := jwt.MapClaims{
		"user_id": "00000000-0000-0000-0000-000000000000", // Mock ID
		"email":   "test@example.com",
		"jti":     jti,
		"exp":     time.Now().Add(time.Hour * 720).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(secret))

	fmt.Println("\n--- MOCK JWT TOKEN ---")
	fmt.Println(tokenString)
	fmt.Println("----------------------")
	fmt.Println("Gunakan token ini di header: Authorization: Bearer <token>")
}
