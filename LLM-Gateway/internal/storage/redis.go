// Package storage provides Redis cache management
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// RedisClient wraps redis.Client with additional functionality
type RedisClient struct {
	client *redis.Client
	config *types.RedisConfig
	logger *utils.Logger
}

// NewRedisClient creates a new Redis client
func NewRedisClient(config *types.RedisConfig, logger *utils.Logger) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", config.Host, config.Port),
		Password: config.Password,
		DB:       config.Database,

		// Connection pool settings
		PoolSize:     10,
		MinIdleConns: 5,

		// Timeouts
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,

		// Retry settings
		MaxRetries:      3,
		MinRetryBackoff: 8 * time.Millisecond,
		MaxRetryBackoff: 512 * time.Millisecond,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Successfully connected to Redis")

	return &RedisClient{
		client: client,
		config: config,
		logger: logger,
	}, nil
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// Ping tests Redis connectivity
func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Set stores a key-value pair with TTL
func (r *RedisClient) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	return r.client.Set(ctx, key, data, ttl).Err()
}

// Get retrieves a value by key
func (r *RedisClient) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("key not found: %s", key)
		}
		return fmt.Errorf("failed to get key %s: %w", key, err)
	}

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return nil
}

// Delete removes a key
func (r *RedisClient) Delete(ctx context.Context, keys ...string) error {
	return r.client.Del(ctx, keys...).Err()
}

// Exists checks if a key exists
func (r *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
	count, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Expire sets TTL for a key
func (r *RedisClient) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return r.client.Expire(ctx, key, ttl).Err()
}

// SessionManager provides session management using Redis
type SessionManager struct {
	redis      *RedisClient
	keyPrefix  string
	defaultTTL time.Duration
}

func NewSessionManager(redis *RedisClient) *SessionManager {
	return &SessionManager{
		redis:      redis,
		keyPrefix:  "session:",
		defaultTTL: 24 * time.Hour,
	}
}

func (s *SessionManager) Set(ctx context.Context, sessionID string, data interface{}) error {
	key := s.keyPrefix + sessionID
	return s.redis.Set(ctx, key, data, s.defaultTTL)
}

func (s *SessionManager) Get(ctx context.Context, sessionID string, dest interface{}) error {
	key := s.keyPrefix + sessionID
	return s.redis.Get(ctx, key, dest)
}

func (s *SessionManager) Delete(ctx context.Context, sessionID string) error {
	key := s.keyPrefix + sessionID
	return s.redis.Delete(ctx, key)
}

func (s *SessionManager) Refresh(ctx context.Context, sessionID string) error {
	key := s.keyPrefix + sessionID
	return s.redis.Expire(ctx, key, s.defaultTTL)
}

// RateLimiter provides rate limiting using Redis
type RateLimiter struct {
	redis     *RedisClient
	keyPrefix string
}

func NewRateLimiter(redis *RedisClient) *RateLimiter {
	return &RateLimiter{
		redis:     redis,
		keyPrefix: "rate_limit:",
	}
}

// Allow checks if a request is allowed under rate limiting
func (rl *RateLimiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	redisKey := rl.keyPrefix + key

	// Use sliding window algorithm with Redis sorted sets
	now := time.Now().UnixNano()
	windowStart := now - window.Nanoseconds()

	pipe := rl.redis.client.Pipeline()

	// Remove expired entries
	pipe.ZRemRangeByScore(ctx, redisKey, "0", fmt.Sprintf("%d", windowStart))

	// Count current requests in window
	countCmd := pipe.ZCard(ctx, redisKey)

	// Add current request
	pipe.ZAdd(ctx, redisKey, redis.Z{Score: float64(now), Member: now})

	// Set expiry for the key
	pipe.Expire(ctx, redisKey, window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to execute rate limit pipeline: %w", err)
	}

	count := countCmd.Val()
	return count < limit, nil
}

// GetCount returns current count for a rate limit key
func (rl *RateLimiter) GetCount(ctx context.Context, key string) (int64, error) {
	redisKey := rl.keyPrefix + key
	return rl.redis.client.ZCard(ctx, redisKey).Result()
}

// Reset resets the rate limit for a key
func (rl *RateLimiter) Reset(ctx context.Context, key string) error {
	redisKey := rl.keyPrefix + key
	return rl.redis.Delete(ctx, redisKey)
}

// CacheManager provides general caching functionality
type CacheManager struct {
	redis     *RedisClient
	keyPrefix string
}

func NewCacheManager(redis *RedisClient, prefix string) *CacheManager {
	return &CacheManager{
		redis:     redis,
		keyPrefix: prefix + ":",
	}
}

// Set caches a value with TTL
func (c *CacheManager) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	cacheKey := c.keyPrefix + key
	return c.redis.Set(ctx, cacheKey, value, ttl)
}

// Get retrieves a cached value
func (c *CacheManager) Get(ctx context.Context, key string, dest interface{}) error {
	cacheKey := c.keyPrefix + key
	return c.redis.Get(ctx, cacheKey, dest)
}

// Delete removes a cached value
func (c *CacheManager) Delete(ctx context.Context, key string) error {
	cacheKey := c.keyPrefix + key
	return c.redis.Delete(ctx, cacheKey)
}

// GetOrSet retrieves a value from cache, or sets it if not found
func (c *CacheManager) GetOrSet(ctx context.Context, key string, dest interface{}, loader func() (interface{}, error), ttl time.Duration) error {
	// Try to get from cache first
	err := c.Get(ctx, key, dest)
	if err == nil {
		return nil // Found in cache
	}

	// Not in cache, load the value
	value, err := loader()
	if err != nil {
		return fmt.Errorf("failed to load value: %w", err)
	}

	// Set in cache
	if err := c.Set(ctx, key, value, ttl); err != nil {
		// Log error but don't fail the request
		c.redis.logger.WithError(err).Warnf("Failed to cache value for key: %s", key)
	}

	// Copy loaded value to destination
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal loaded value: %w", err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("failed to unmarshal loaded value: %w", err)
	}

	return nil
}

// InvalidatePattern invalidates all keys matching a pattern
func (c *CacheManager) InvalidatePattern(ctx context.Context, pattern string) error {
	fullPattern := c.keyPrefix + pattern

	iter := c.redis.client.Scan(ctx, 0, fullPattern, 0).Iterator()
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("failed to scan keys: %w", err)
	}

	if len(keys) > 0 {
		return c.redis.client.Del(ctx, keys...).Err()
	}

	return nil
}

// ConfigCache provides configuration caching
type ConfigCache struct {
	*CacheManager
}

func NewConfigCache(redis *RedisClient) *ConfigCache {
	return &ConfigCache{
		CacheManager: NewCacheManager(redis, "config"),
	}
}

// ProviderCache provides provider information caching
type ProviderCache struct {
	*CacheManager
}

func NewProviderCache(redis *RedisClient) *ProviderCache {
	return &ProviderCache{
		CacheManager: NewCacheManager(redis, "provider"),
	}
}

// UserCache provides user information caching
type UserCache struct {
	*CacheManager
}

func NewUserCache(redis *RedisClient) *UserCache {
	return &UserCache{
		CacheManager: NewCacheManager(redis, "user"),
	}
}

// APIKeyCache provides API key validation caching
type APIKeyCache struct {
	*CacheManager
}

func NewAPIKeyCache(redis *RedisClient) *APIKeyCache {
	return &APIKeyCache{
		CacheManager: NewCacheManager(redis, "apikey"),
	}
}

// CacheInvalidator provides cache invalidation functionality
type CacheInvalidator struct {
	redis         *RedisClient
	configCache   *ConfigCache
	providerCache *ProviderCache
	userCache     *UserCache
	apiKeyCache   *APIKeyCache
}

func NewCacheInvalidator(redis *RedisClient) *CacheInvalidator {
	return &CacheInvalidator{
		redis:         redis,
		configCache:   NewConfigCache(redis),
		providerCache: NewProviderCache(redis),
		userCache:     NewUserCache(redis),
		apiKeyCache:   NewAPIKeyCache(redis),
	}
}

// InvalidateUser invalidates all user-related caches
func (ci *CacheInvalidator) InvalidateUser(ctx context.Context, userID uint) error {
	userKey := fmt.Sprintf("user:%d", userID)
	if err := ci.userCache.Delete(ctx, userKey); err != nil {
		return err
	}

	// Also invalidate user's API keys
	return ci.apiKeyCache.InvalidatePattern(ctx, fmt.Sprintf("user:%d:*", userID))
}

// InvalidateProvider invalidates provider-related caches
func (ci *CacheInvalidator) InvalidateProvider(ctx context.Context, providerID uint) error {
	return ci.providerCache.InvalidatePattern(ctx, fmt.Sprintf("*:%d:*", providerID))
}

// InvalidateConfig invalidates configuration caches
func (ci *CacheInvalidator) InvalidateConfig(ctx context.Context) error {
	return ci.configCache.InvalidatePattern(ctx, "*")
}

// Global Redis client instance
var DefaultRedis *RedisClient

// InitDefaultRedis initializes the default Redis client
func InitDefaultRedis(config *types.RedisConfig, logger *utils.Logger) error {
	client, err := NewRedisClient(config, logger)
	if err != nil {
		return err
	}

	DefaultRedis = client
	return nil
}

// GetRedis returns the default Redis client
func GetRedis() *RedisClient {
	return DefaultRedis
}
