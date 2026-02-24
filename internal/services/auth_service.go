package services

import (
	"walletx-be/internal/models"
	"walletx-be/internal/repository"
)

// DTO (Data Transfer Object) untuk menerima input dari Handler
type GoogleAuthInput struct {
	GoogleID string `json:"google_id" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Name     string `json:"name" binding:"required"`
	Picture  string `json:"picture"`
}

type AuthService struct {
	userRepo repository.UserRepository
}

func NewAuthService(userRepo repository.UserRepository) *AuthService {
	return &AuthService{userRepo: userRepo}
}

// ProcessGoogleAuth menangani logic Register/Login
func (s *AuthService) ProcessGoogleAuth(input GoogleAuthInput) (*models.User, bool, error) {
	// 1. Cek apakah user sudah ada
	existingUser, err := s.userRepo.FindByGoogleID(input.GoogleID)
	if err != nil {
		return nil, false, err
	}

	// 2. Jika SUDAH ADA, kembalikan data user tersebut (Status = Login)
	if existingUser != nil {
		return existingUser, false, nil // false berarti "bukan user baru"
	}

	// 3. Jika BELUM ADA, buat objek User baru (Status = Register)
	newUser := &models.User{
		GoogleID: input.GoogleID,
		Email:    input.Email,
		Name:     input.Name,
		Picture:  input.Picture,
	}

	err = s.userRepo.Create(newUser)
	if err != nil {
		return nil, false, err
	}

	return newUser, true, nil // true berarti "berhasil register akun baru"
}
