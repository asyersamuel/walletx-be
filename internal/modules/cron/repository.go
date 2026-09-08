package cron

import "gorm.io/gorm"

// HealthRepository is a lightweight DB health-check capability.
type HealthRepository interface {
	Ping() error
}

type healthRepository struct {
	db *gorm.DB
}

func NewHealthRepository(db *gorm.DB) HealthRepository {
	return &healthRepository{db: db}
}

func (r *healthRepository) Ping() error {
	return r.db.Exec("SELECT 1").Error
}
