package infra

import (
	"fmt"
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"hhwtrade.com/internal/config"
	"hhwtrade.com/internal/model"
)

type PostgresClient struct {
	DB *gorm.DB
}

func NewPostgresClient(cfg config.DatabaseConfig) (*PostgresClient, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=%s",
		cfg.Host, cfg.User, cfg.Password, cfg.DBName, cfg.Port, cfg.SSLMode, cfg.TimeZone)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   cfg.TablePrefix,
			SingularTable: false,
			// NoLowerCase:   true, // Preserve PascalCase for columns
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Println("Database connected successfully")

	// Auto Migrate
	if err := db.AutoMigrate(
		&model.User{},
		&model.Subscription{},
		&model.Future{},
		&model.Strategy{},
		&model.Order{},
		&model.Trade{},
		&model.OrderLog{},
		&model.Position{},
	); err != nil {
		log.Printf("Warning: AutoMigrate failed: %v", err)
	}

	return &PostgresClient{DB: db}, nil
}
