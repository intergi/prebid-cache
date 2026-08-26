package backends

import (
	"context"
	"crypto/tls"
	"strconv"
	"time"

	"github.com/prebid/prebid-cache/config"
	"github.com/prebid/prebid-cache/utils"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

// RedisDB is an interface that helps us communicate with an instance of a
// Redis database. Its implementation is intended to use the "github.com/go-redis/redis"
// client
type RedisDB interface {
	Get(ctx context.Context, key string) (string, error)
	Put(ctx context.Context, key string, value string, ttlSeconds int) (bool, error)
}

// RedisDBClient is a wrapper for the Redis client that implements
// the RedisDB interface
type RedisDBClient struct {
	client *redis.Client
}

// Get returns the value associated with the provided `key` parameter
func (db RedisDBClient) Get(ctx context.Context, key string) (string, error) {
	return db.client.Get(ctx, key).Result()
}

// Put will set 'key' to hold string 'value' if 'key' does not exist in the redis storage.
// When key already holds a value, no operation is performed. That's the reason this adapter
// uses the 'github.com/go-redis/redis's library SetNX. SetNX is short for "SET if Not eXists".
func (db RedisDBClient) Put(ctx context.Context, key, value string, ttlSeconds int) (bool, error) {
	return db.client.SetNX(ctx, key, value, time.Duration(ttlSeconds)*time.Second).Result()
}

// RedisBackend when initialized will instantiate and configure the Redis client. It implements
// the Backend interface.
type RedisBackend struct {
	cfg    config.Redis
	client RedisDB
}

func newRedisOptions(cfg config.Redis) (*redis.Options, error) {
	var options *redis.Options
	var err error

	if cfg.URL != "" {
		options, err = redis.ParseURL(cfg.URL)
		if err != nil {
			return nil, err
		}
	} else {
		options = &redis.Options{}
	}

	if cfg.Host != "" || cfg.Port > 0 {
		options.Addr = cfg.Host + ":" + strconv.Itoa(cfg.Port)
	}
	if cfg.Password != "" {
		options.Password = cfg.Password
	}
	if cfg.Db != 0 {
		options.DB = cfg.Db
	}
	if cfg.PoolSize > 0 {
		options.PoolSize = cfg.PoolSize
	}
	if cfg.ReadBufferSize > 0 {
		options.ReadBufferSize = cfg.ReadBufferSize
	}
	if cfg.WriteBufferSize > 0 {
		options.WriteBufferSize = cfg.WriteBufferSize
	}
	if cfg.MaxRetries > 0 {
		options.MaxRetries = cfg.MaxRetries
	}
	if cfg.DialerRetries > 0 {
		options.DialerRetries = cfg.DialerRetries
	}
	if cfg.DialTimeoutSeconds > 0 {
		options.DialTimeout = time.Duration(cfg.DialTimeoutSeconds) * time.Second
	}
	if cfg.ReadTimeoutSeconds > 0 {
		options.ReadTimeout = time.Duration(cfg.ReadTimeoutSeconds) * time.Second
	}
	if cfg.WriteTimeoutSeconds > 0 {
		options.WriteTimeout = time.Duration(cfg.WriteTimeoutSeconds) * time.Second
	}
	if cfg.MaxIdleConns > 0 {
		options.MaxIdleConns = cfg.MaxIdleConns
	}
	if cfg.MaxActiveConns > 0 {
		options.MaxActiveConns = cfg.MaxActiveConns
	}

	options.PoolFIFO = true
	options.ConnMaxLifetimeJitter = 1 * time.Minute
	options.ConnMaxIdleTime = 1 * time.Minute
	options.ConnMaxLifetime = 5 * time.Minute
	options.DisableIdentity = true

	if cfg.TLS.Enabled {
		if options.TLSConfig == nil {
			options.TLSConfig = &tls.Config{}
		}
		options.TLSConfig.InsecureSkipVerify = cfg.TLS.InsecureSkipVerify
	}

	return options, nil
}

// NewRedisBackend initializes the redis client and pings to make sure connection was successful
func NewRedisBackend(cfg config.Redis, ctx context.Context) *RedisBackend {
	options, err := newRedisOptions(cfg)
	if err != nil {
		log.Fatalf("Error creating Redis backend: %v", err)
	}

	redisClient := RedisDBClient{client: redis.NewClient(options)}

	_, err = redisClient.client.Ping(ctx).Result()

	if err != nil {
		log.Fatalf("Error creating Redis backend: %v", err)
		panic("RedisBackend failure. This shouldn't happen.")
	}

	log.Infof("Connected to Redis at %s", options.Addr)

	return &RedisBackend{
		cfg:    cfg,
		client: redisClient,
	}
}

// Get calls the Redis client to return the value associated with the provided `key`
// parameter and interprets its response. A `Nil` error reply of the Redis client means
// the `key` does not exist.
func (b *RedisBackend) Get(ctx context.Context, key string) (string, error) {
	res, err := b.client.Get(ctx, key)

	if err == redis.Nil {
		err = utils.NewPBCError(utils.KEY_NOT_FOUND)
	}

	return res, err
}

// Put writes the `value` under the provided `key` in the Redis storage server. Because the backend
// implementation of Put calls SetNX(item *Item), a `false` return value is interpreted as the data
// not being written because the `key` already holds a value, and a RecordExistsError is returned
func (b *RedisBackend) Put(ctx context.Context, key string, value string, ttlSeconds int) error {

	success, err := b.client.Put(ctx, key, value, ttlSeconds)
	if err != nil && err != redis.Nil {
		return err
	}
	if !success {
		return utils.NewPBCError(utils.RECORD_EXISTS)
	}
	return nil
}
