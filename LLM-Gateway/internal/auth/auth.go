// Package auth provides authentication and authorization functionality
package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/llm-gateway/gateway/internal/storage"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// Claims represents JWT claims
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	IsAdmin  bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

// AuthService provides authentication services
type AuthService struct {
	config     *types.AuthConfig
	logger     *utils.Logger
	userRepo   *storage.UserRepository
	apiKeyRepo *storage.APIKeyRepository
	jwtSecret  []byte
}

// NewAuthService creates a new authentication service
func NewAuthService(config *types.AuthConfig, logger *utils.Logger, db *storage.Database) *AuthService {
	return &AuthService{
		config:     config,
		logger:     logger,
		userRepo:   db.UserRepo(),
		apiKeyRepo: db.APIKeyRepo(),
		jwtSecret:  []byte(config.JWTSecret),
	}
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresIn int64     `json:"expires_in"`
	User      *UserInfo `json:"user"`
}

// UserInfo represents user information for responses
type UserInfo struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	IsAdmin  bool   `json:"is_admin"`
}

// Login authenticates a user and returns a JWT token
func (a *AuthService) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	// Get user by username
	user, err := a.userRepo.GetByUsername(req.Username)
	if err != nil {
		a.logger.LogAuthFailure(ctx, "user_not_found", "", "")
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check if user is active
	if !user.IsActive {
		a.logger.LogAuthFailure(ctx, "user_inactive", "", "")
		return nil, fmt.Errorf("account is inactive")
	}

	// Verify password
	if err := utils.CheckPassword(req.Password, user.Password); err != nil {
		a.logger.LogAuthFailure(ctx, "invalid_password", "", "")
		return nil, fmt.Errorf("invalid credentials")
	}

	// Generate JWT token
	token, expiresIn, err := a.generateJWT(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	a.logger.WithUserID(fmt.Sprintf("%d", user.ID)).Info("User logged in successfully")

	return &LoginResponse{
		Token:     token,
		ExpiresIn: expiresIn,
		User: &UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			IsAdmin:  user.IsAdmin,
		},
	}, nil
}

// Register creates a new user account
func (a *AuthService) Register(ctx context.Context, req *types.RegisterRequest) (*types.RegisterResponse, error) {
	// Check if username already exists
	existingUser, err := a.userRepo.GetByUsername(req.Username)
	if err == nil && existingUser != nil {
		return nil, fmt.Errorf("username already exists")
	}

	// Check if email already exists
	existingUser, err = a.userRepo.GetByEmail(req.Email)
	if err == nil && existingUser != nil {
		return nil, fmt.Errorf("email already exists")
	}

	// Hash password
	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create new user
	user := &storage.User{
		Username: req.Username,
		Email:    req.Email,
		Password: hashedPassword,
		FullName: req.FullName,
		IsActive: true,
		IsAdmin:  false, // New users are not admin by default
	}

	// Save user to database
	if err := a.userRepo.Create(user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	a.logger.WithUserID(fmt.Sprintf("%d", user.ID)).Info("User registered successfully")

	return &types.RegisterResponse{
		UserID:   user.ID,
		Username: user.Username,
		Email:    user.Email,
		FullName: user.FullName,
		Message:  "User registered successfully",
	}, nil
}

// generateJWT generates a JWT token for a user
func (a *AuthService) generateJWT(user *storage.User) (string, int64, error) {
	expirationTime := time.Now().Add(a.config.JWTExpiration)

	claims := &Claims{
		UserID:   user.ID,
		Username: user.Username,
		Email:    user.Email,
		IsAdmin:  user.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "llm-gateway",
			Subject:   fmt.Sprintf("%d", user.ID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(a.jwtSecret)
	if err != nil {
		return "", 0, err
	}

	return tokenString, int64(a.config.JWTExpiration.Seconds()), nil
}

// ValidateJWT validates a JWT token and returns the claims
func (a *AuthService) ValidateJWT(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return a.jwtSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}

// ValidateAPIKey validates an API key and returns the associated user
func (a *AuthService) ValidateAPIKey(ctx context.Context, apiKey string) (*storage.User, *storage.APIKey, error) {
	// Get API key from database
	keyRecord, err := a.apiKeyRepo.GetByKey(apiKey)
	if err != nil {
		a.logger.LogAuthFailure(ctx, "api_key_not_found", "", "")
		return nil, nil, fmt.Errorf("invalid API key")
	}

	// Check if API key is active
	if !keyRecord.IsActive {
		a.logger.LogAuthFailure(ctx, "api_key_inactive", "", "")
		return nil, nil, fmt.Errorf("API key is inactive")
	}

	// Check if API key has expired
	if keyRecord.ExpiresAt != nil && keyRecord.ExpiresAt.Before(time.Now()) {
		a.logger.LogAuthFailure(ctx, "api_key_expired", "", "")
		return nil, nil, fmt.Errorf("API key has expired")
	}

	// Check if user is active
	if !keyRecord.User.IsActive {
		a.logger.LogAuthFailure(ctx, "user_inactive", "", "")
		return nil, nil, fmt.Errorf("user account is inactive")
	}

	// Update last used timestamp
	if err := a.apiKeyRepo.UpdateLastUsed(keyRecord.ID); err != nil {
		a.logger.WithError(err).Warn("Failed to update API key last used timestamp")
	}

	return &keyRecord.User, keyRecord, nil
}

// CreateAPIKeyRequest represents a request to create an API key
type CreateAPIKeyRequest struct {
	Name      string     `json:"name" binding:"required"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreateAPIKeyResponse represents a response for API key creation
type CreateAPIKeyResponse struct {
	ID        uint       `json:"id"`
	Name      string     `json:"name"`
	Key       string     `json:"key"` // Only returned on creation
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreateAPIKey creates a new API key for a user
func (a *AuthService) CreateAPIKey(ctx context.Context, userID uint, req *CreateAPIKeyRequest) (*CreateAPIKeyResponse, error) {
	// Generate API key
	apiKey, err := utils.GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Hash API key for storage
	keyHash := utils.HashAPIKey(apiKey)

	// Create API key record
	keyRecord := &storage.APIKey{
		UserID:    userID,
		Name:      req.Name,
		Key:       apiKey,
		KeyHash:   keyHash,
		IsActive:  true,
		ExpiresAt: req.ExpiresAt,
	}

	if err := a.apiKeyRepo.Create(keyRecord); err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	a.logger.WithUserID(fmt.Sprintf("%d", userID)).Info("API key created successfully")

	return &CreateAPIKeyResponse{
		ID:        keyRecord.ID,
		Name:      keyRecord.Name,
		Key:       apiKey, // Only returned on creation
		ExpiresAt: keyRecord.ExpiresAt,
		CreatedAt: keyRecord.CreatedAt,
	}, nil
}

// ListAPIKeys returns all API keys for a user (without the actual key values)
func (a *AuthService) ListAPIKeys(ctx context.Context, userID uint) ([]storage.APIKey, error) {
	apiKeys, err := a.apiKeyRepo.GetByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get API keys: %w", err)
	}

	// Remove sensitive information
	for i := range apiKeys {
		apiKeys[i].Key = ""
		apiKeys[i].KeyHash = ""
	}

	return apiKeys, nil
}

// RevokeAPIKey revokes an API key
func (a *AuthService) RevokeAPIKey(ctx context.Context, userID, keyID uint) error {
	// Get API key to verify ownership
	apiKeys, err := a.apiKeyRepo.GetByUserID(userID)
	if err != nil {
		return fmt.Errorf("failed to get API keys: %w", err)
	}

	var targetKey *storage.APIKey
	for _, key := range apiKeys {
		if key.ID == keyID {
			targetKey = &key
			break
		}
	}

	if targetKey == nil {
		return fmt.Errorf("API key not found or not owned by user")
	}

	// Deactivate the API key
	targetKey.IsActive = false
	if err := a.apiKeyRepo.Update(targetKey); err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	a.logger.WithUserID(fmt.Sprintf("%d", userID)).Info("API key revoked successfully")

	return nil
}

// RefreshToken creates a new JWT token from an existing valid token
func (a *AuthService) RefreshToken(ctx context.Context, tokenString string) (*LoginResponse, error) {
	// Validate current token
	claims, err := a.ValidateJWT(tokenString)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Get updated user information
	user, err := a.userRepo.GetByID(claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	if !user.IsActive {
		return nil, fmt.Errorf("user account is inactive")
	}

	// Generate new token
	newToken, expiresIn, err := a.generateJWT(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new token: %w", err)
	}

	return &LoginResponse{
		Token:     newToken,
		ExpiresIn: expiresIn,
		User: &UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			IsAdmin:  user.IsAdmin,
		},
	}, nil
}

// GetUserByToken retrieves user information from a JWT token
func (a *AuthService) GetUserByToken(ctx context.Context, tokenString string) (*storage.User, error) {
	claims, err := a.ValidateJWT(tokenString)
	if err != nil {
		return nil, err
	}

	user, err := a.userRepo.GetByID(claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	if !user.IsActive {
		return nil, fmt.Errorf("user account is inactive")
	}

	return user, nil
}

// GetUserFromContext retrieves user information from context
func (a *AuthService) GetUserFromContext(ctx context.Context) *storage.User {
	if userCtx := ctx.Value("user"); userCtx != nil {
		if user, ok := userCtx.(*storage.User); ok {
			return user
		}
	}
	return nil
}

// Global auth service instance
var DefaultAuthService *AuthService

// InitDefaultAuthService initializes the default auth service
func InitDefaultAuthService(config *types.AuthConfig, logger *utils.Logger, db *storage.Database) {
	DefaultAuthService = NewAuthService(config, logger, db)
}

// GetAuthService returns the default auth service
func GetAuthService() *AuthService {
	return DefaultAuthService
}
