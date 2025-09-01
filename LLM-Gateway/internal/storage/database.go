// Package storage provides database connection and management
package storage

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// Database represents the database connection manager
type Database struct {
	DB     *gorm.DB
	config *types.DatabaseConfig
	logger *utils.Logger
}

// NewDatabase creates a new database connection
func NewDatabase(config *types.DatabaseConfig, log *utils.Logger) (*Database, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Shanghai",
		config.Host,
		config.Port,
		config.Username,
		config.Password,
		config.Database,
	)

	// Configure GORM logger
	gormLogger := logger.New(
		log, // Use our custom logger
		logger.Config{
			SlowThreshold:             time.Second,   // Slow SQL threshold
			LogLevel:                  logger.Silent, // Log level
			IgnoreRecordNotFoundError: true,          // Ignore ErrRecordNotFound error for logger
			Colorful:                  false,         // Disable color
		},
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying SQL DB for connection pool configuration
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying SQL DB: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	database := &Database{
		DB:     db,
		config: config,
		logger: log,
	}

	// Test connection
	if err := database.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info("Successfully connected to PostgreSQL database")

	return database, nil
}

// Ping tests the database connection
func (d *Database) Ping() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return sqlDB.PingContext(ctx)
}

// Close closes the database connection
func (d *Database) Close() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// AutoMigrate runs database migrations
func (d *Database) AutoMigrate() error {
	d.logger.Info("Starting database migration")

	// List of models to migrate
	models := []interface{}{
		&User{},
		&APIKey{},
		&Quota{},
		&Provider{},
		&Model{},
		&Request{},
		&ProviderHealth{},
		&RateLimitRecord{},
		&ConfigSetting{},
	}

	for _, model := range models {
		if err := d.DB.AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to migrate model %T: %w", model, err)
		}
	}

	d.logger.Info("Database migration completed successfully")
	return nil
}

// CreateDefaultData creates default data for the system
func (d *Database) CreateDefaultData() error {
	d.logger.Info("Creating default data")

	// Create default admin user if not exists
	var adminUser User
	result := d.DB.Where("username = ?", "admin").First(&adminUser)
	if result.Error == gorm.ErrRecordNotFound {
		hashedPassword, err := utils.HashPassword("admin123") // Default password
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}

		adminUser = User{
			Username: "admin",
			Email:    "admin@example.com",
			Password: hashedPassword,
			IsActive: true,
			IsAdmin:  true,
		}

		if err := d.DB.Create(&adminUser).Error; err != nil {
			return fmt.Errorf("failed to create admin user: %w", err)
		}

		d.logger.Info("Created default admin user")
	}

	// Create default system configuration
	defaultConfigs := []ConfigSetting{
		{Key: "system.max_requests_per_minute", Value: "1000", Type: "int", Category: "rate_limit"},
		{Key: "system.max_tokens_per_request", Value: "4096", Type: "int", Category: "limits"},
		{Key: "system.default_provider_timeout", Value: "30", Type: "int", Category: "provider"},
		{Key: "system.enable_request_logging", Value: "true", Type: "bool", Category: "logging"},
		{Key: "system.enable_provider_health_check", Value: "true", Type: "bool", Category: "monitoring"},
	}

	for _, config := range defaultConfigs {
		var existingConfig ConfigSetting
		result := d.DB.Where("key = ?", config.Key).First(&existingConfig)
		if result.Error == gorm.ErrRecordNotFound {
			if err := d.DB.Create(&config).Error; err != nil {
				d.logger.WithError(err).Warnf("Failed to create default config: %s", config.Key)
			}
		}
	}

	d.logger.Info("Default data creation completed")
	return nil
}

// Repository interfaces and implementations

// UserRepository provides user data access methods
type UserRepository struct {
	db *gorm.DB
}

func (d *Database) UserRepo() *UserRepository {
	return &UserRepository{db: d.DB}
}

func (r *UserRepository) Create(user *User) error {
	return r.db.Create(user).Error
}

func (r *UserRepository) GetByID(id uint) (*User, error) {
	var user User
	err := r.db.Preload("APIKeys").Preload("Quotas").First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) GetByUsername(username string) (*User, error) {
	var user User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) GetByEmail(email string) (*User, error) {
	var user User
	err := r.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) Update(user *User) error {
	return r.db.Save(user).Error
}

func (r *UserRepository) Delete(id uint) error {
	return r.db.Delete(&User{}, id).Error
}

func (r *UserRepository) List(offset, limit int) ([]User, error) {
	var users []User
	err := r.db.Offset(offset).Limit(limit).Find(&users).Error
	return users, err
}

// APIKeyRepository provides API key data access methods
type APIKeyRepository struct {
	db *gorm.DB
}

func (d *Database) APIKeyRepo() *APIKeyRepository {
	return &APIKeyRepository{db: d.DB}
}

func (r *APIKeyRepository) Create(apiKey *APIKey) error {
	return r.db.Create(apiKey).Error
}

func (r *APIKeyRepository) GetByKey(key string) (*APIKey, error) {
	var apiKey APIKey
	err := r.db.Preload("User").Where("key = ? AND is_active = ?", key, true).First(&apiKey).Error
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}

func (r *APIKeyRepository) GetByUserID(userID uint) ([]APIKey, error) {
	var apiKeys []APIKey
	err := r.db.Where("user_id = ?", userID).Find(&apiKeys).Error
	return apiKeys, err
}

func (r *APIKeyRepository) Update(apiKey *APIKey) error {
	return r.db.Save(apiKey).Error
}

func (r *APIKeyRepository) UpdateLastUsed(keyID uint) error {
	now := time.Now()
	return r.db.Model(&APIKey{}).Where("id = ?", keyID).Update("last_used_at", &now).Error
}

func (r *APIKeyRepository) Delete(id uint) error {
	return r.db.Delete(&APIKey{}, id).Error
}

// RequestRepository provides request logging data access methods
type RequestRepository struct {
	db *gorm.DB
}

func (d *Database) RequestRepo() *RequestRepository {
	return &RequestRepository{db: d.DB}
}

func (r *RequestRepository) Create(request *Request) error {
	return r.db.Create(request).Error
}

func (r *RequestRepository) GetByRequestID(requestID string) (*Request, error) {
	var request Request
	err := r.db.Preload("User").Preload("Provider").Where("request_id = ?", requestID).First(&request).Error
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func (r *RequestRepository) GetByUserID(userID uint, offset, limit int) ([]Request, error) {
	var requests []Request
	err := r.db.Where("user_id = ?", userID).Offset(offset).Limit(limit).Order("created_at DESC").Find(&requests).Error
	return requests, err
}

func (r *RequestRepository) GetStats(userID *uint, startTime, endTime time.Time) (map[string]interface{}, error) {
	var stats struct {
		TotalRequests   int64   `json:"total_requests"`
		SuccessRequests int64   `json:"success_requests"`
		TotalTokens     int64   `json:"total_tokens"`
		TotalCost       float64 `json:"total_cost"`
		AvgResponseTime float64 `json:"avg_response_time"`
	}

	query := r.db.Model(&Request{}).Where("created_at BETWEEN ? AND ?", startTime, endTime)
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}

	err := query.Select(
		"COUNT(*) as total_requests",
		"COUNT(CASE WHEN status_code < 400 THEN 1 END) as success_requests",
		"SUM(total_tokens) as total_tokens",
		"SUM(cost) as total_cost",
		"AVG(response_time) as avg_response_time",
	).Scan(&stats).Error

	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"total_requests":    stats.TotalRequests,
		"success_requests":  stats.SuccessRequests,
		"total_tokens":      stats.TotalTokens,
		"total_cost":        stats.TotalCost,
		"avg_response_time": stats.AvgResponseTime,
	}

	return result, nil
}

// Global database instance
var DefaultDB *Database

// InitDefaultDatabase initializes the default database connection
func InitDefaultDatabase(config *types.DatabaseConfig, logger *utils.Logger) error {
	db, err := NewDatabase(config, logger)
	if err != nil {
		return err
	}

	DefaultDB = db

	// Run migrations
	if err := db.AutoMigrate(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Create default data
	if err := db.CreateDefaultData(); err != nil {
		return fmt.Errorf("failed to create default data: %w", err)
	}

	return nil
}

// GetDB returns the default database instance
func GetDB() *Database {
	return DefaultDB
}
