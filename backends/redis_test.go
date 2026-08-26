package backends

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prebid/prebid-cache/config"
	"github.com/prebid/prebid-cache/utils"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestRedisClientGet(t *testing.T) {
	redisBackend := &RedisBackend{}

	type testInput struct {
		redisClient RedisDB
		key         string
	}

	type testExpectedValues struct {
		value string
		err   error
	}

	testCases := []struct {
		desc     string
		in       testInput
		expected testExpectedValues
	}{
		{
			desc: "RedisBackend.Get() throws a redis.Nil error",
			in: testInput{
				redisClient: FakeRedisClient{
					Success:     false,
					ServerError: redis.Nil,
				},
				key: "someKeyThatWontBeFound",
			},
			expected: testExpectedValues{
				value: "",
				err:   utils.NewPBCError(utils.KEY_NOT_FOUND),
			},
		},
		{
			desc: "RedisBackend.Get() throws an error different from redis.Nil",
			in: testInput{
				redisClient: FakeRedisClient{
					Success:     false,
					ServerError: errors.New("some other get error"),
				},
				key: "someKey",
			},
			expected: testExpectedValues{
				value: "",
				err:   errors.New("some other get error"),
			},
		},
		{
			desc: "RedisBackend.Get() doesn't throw an error",
			in: testInput{
				redisClient: FakeRedisClient{
					Success:    true,
					StoredData: map[string]string{"defaultKey": "aValue"},
				},
				key: "defaultKey",
			},
			expected: testExpectedValues{
				value: "aValue",
				err:   nil,
			},
		},
	}

	for _, tt := range testCases {
		redisBackend.client = tt.in.redisClient

		// Run test
		actualValue, actualErr := redisBackend.Get(context.Background(), tt.in.key)

		// Assertions
		assert.Equal(t, tt.expected.value, actualValue, tt.desc)
		assert.Equal(t, tt.expected.err, actualErr, tt.desc)
	}
}

func TestRedisClientPut(t *testing.T) {
	redisBackend := &RedisBackend{}

	type testInput struct {
		redisClient  RedisDB
		key          string
		valueToStore string
		ttl          int
	}

	type testExpectedValues struct {
		writtenValue   string
		redisClientErr error
	}

	testCases := []struct {
		desc     string
		in       testInput
		expected testExpectedValues
	}{
		{
			desc: "Try to overwrite already existing key. From redis client documentation, SetNX returns 'false' because no operation is performed",
			in: testInput{
				redisClient: FakeRedisClient{
					Success:     false,
					StoredData:  map[string]string{"key": "original value"},
					ServerError: redis.Nil,
				},
				key:          "key",
				valueToStore: "overwrite value",
				ttl:          10,
			},
			expected: testExpectedValues{
				redisClientErr: utils.NewPBCError(utils.RECORD_EXISTS),
				writtenValue:   "original value",
			},
		},
		{
			desc: "When key does not exist, redis.Nil is returned. Other errors should be interpreted as a server side error. Expect error.",
			in: testInput{
				redisClient: FakeRedisClient{
					Success:     true,
					StoredData:  map[string]string{},
					ServerError: errors.New("A Redis client side error"),
				},
				key:          "someKey",
				valueToStore: "someValue",
				ttl:          10,
			},
			expected: testExpectedValues{
				redisClientErr: errors.New("A Redis client side error"),
			},
		},
		{
			desc: "In Redis, a zero ttl value means no expiration. Expect value to be successfully set",
			in: testInput{
				redisClient: FakeRedisClient{
					StoredData:  map[string]string{},
					Success:     true,
					ServerError: redis.Nil,
				},
				key:          "defaultKey",
				valueToStore: "aValue",
				ttl:          0,
			},
			expected: testExpectedValues{
				writtenValue: "aValue",
			},
		},
		{
			desc: "RedisBackend.Put() successful, no need to set defaultTTL because ttl is greater than zero",
			in: testInput{
				redisClient: FakeRedisClient{
					StoredData:  map[string]string{},
					Success:     true,
					ServerError: redis.Nil,
				},
				key:          "defaultKey",
				valueToStore: "aValue",
				ttl:          1,
			},
			expected: testExpectedValues{
				writtenValue: "aValue",
			},
		},
	}

	for _, tt := range testCases {
		// Assign redis backend client
		redisBackend.client = tt.in.redisClient

		// Run test
		actualErr := redisBackend.Put(context.Background(), tt.in.key, tt.in.valueToStore, tt.in.ttl)

		// Assertions
		assert.Equal(t, tt.expected.redisClientErr, actualErr, tt.desc)

		// Put error
		assert.Equal(t, tt.expected.redisClientErr, actualErr, tt.desc)

		if actualErr == nil || actualErr == utils.NewPBCError(utils.RECORD_EXISTS) {
			// Either a value was inserted successfully or the record already existed.
			// Assert data in the backend
			storage, ok := tt.in.redisClient.(FakeRedisClient)
			assert.True(t, ok, tt.desc)
			assert.Equal(t, tt.expected.writtenValue, storage.StoredData[tt.in.key], tt.desc)
		}
	}
}

func TestNewRedisOptions(t *testing.T) {
	testCases := []struct {
		desc          string
		cfg           config.Redis
		expectError   bool
		expectedAddr  string
		expectedPass  string
		expectedDB    int
		expectedPool  int
		expectedTLS   bool
		expectedInsec bool
	}{
		{
			desc:         "URL only",
			cfg:          config.Redis{URL: "redis://:mypass@myhost:6380/2"},
			expectError:  false,
			expectedAddr: "myhost:6380",
			expectedPass: "mypass",
			expectedDB:   2,
		},
		{
			desc:         "URL with cfg overrides",
			cfg:          config.Redis{URL: "redis://:mypass@myhost:6380/2", Host: "overridehost", Port: 6379, Password: "newpass", Db: 5, PoolSize: 10},
			expectError:  false,
			expectedAddr: "overridehost:6379",
			expectedPass: "newpass",
			expectedDB:   5,
			expectedPool: 10,
		},
		{
			desc:         "No URL, traditional config",
			cfg:          config.Redis{Host: "localhost", Port: 6379, Password: "secret", Db: 1, PoolSize: 10},
			expectError:  false,
			expectedAddr: "localhost:6379",
			expectedPass: "secret",
			expectedDB:   1,
			expectedPool: 10,
		},
		{
			desc:        "Invalid URL",
			cfg:         config.Redis{URL: "://invalid-url"},
			expectError: true,
		},
		{
			desc:          "TLS enabled",
			cfg:           config.Redis{URL: "redis://myhost:6379", TLS: config.RedisTLS{Enabled: true, InsecureSkipVerify: true}},
			expectError:   false,
			expectedAddr:  "myhost:6379",
			expectedTLS:   true,
			expectedInsec: true,
		},
		{
			desc:         "rediss scheme URL (TLS enabled via scheme)",
			cfg:          config.Redis{URL: "rediss://:mypass@myhost:6379/0"},
			expectError:  false,
			expectedAddr: "myhost:6379",
			expectedPass: "mypass",
			expectedDB:   0,
			expectedTLS:  true,
		},
		{
			desc:          "rediss scheme URL with TLS config override (InsecureSkipVerify)",
			cfg:           config.Redis{URL: "rediss://:mypass@myhost:6379/0", TLS: config.RedisTLS{Enabled: true, InsecureSkipVerify: true}},
			expectError:   false,
			expectedAddr:  "myhost:6379",
			expectedPass:  "mypass",
			expectedDB:    0,
			expectedTLS:   true,
			expectedInsec: true,
		},
	}

	for _, tt := range testCases {
		options, err := newRedisOptions(tt.cfg)
		if tt.expectError {
			assert.Error(t, err, tt.desc)
			assert.Nil(t, options, tt.desc)
		} else {
			assert.NoError(t, err, tt.desc)
			assert.NotNil(t, options, tt.desc)
			assert.Equal(t, tt.expectedAddr, options.Addr, tt.desc)
			assert.Equal(t, tt.expectedPass, options.Password, tt.desc)
			assert.Equal(t, tt.expectedDB, options.DB, tt.desc)
			if tt.expectedPool > 0 {
				assert.Equal(t, tt.expectedPool, options.PoolSize, tt.desc)
			}
			assert.True(t, options.PoolFIFO, tt.desc)
			assert.True(t, options.DisableIdentity, tt.desc)
			assert.Equal(t, 1*time.Minute, options.ConnMaxLifetimeJitter, tt.desc)
			assert.Equal(t, 1*time.Minute, options.ConnMaxIdleTime, tt.desc)
			assert.Equal(t, 5*time.Minute, options.ConnMaxLifetime, tt.desc)

			if tt.expectedTLS {
				assert.NotNil(t, options.TLSConfig, tt.desc)
				assert.Equal(t, tt.expectedInsec, options.TLSConfig.InsecureSkipVerify, tt.desc)
			}
		}
	}
}
