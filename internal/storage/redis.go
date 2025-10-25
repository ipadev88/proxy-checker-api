package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/proxy-checker-api/internal/snapshot"
	"github.com/redis/go-redis/v9"
)

type RedisStorage struct {
	client *redis.Client
	key    string
}

func NewRedisStorage(addr string) (*RedisStorage, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     "",
		DB:           0,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &RedisStorage{
		client: client,
		key:    "proxychecker:snapshot",
	}, nil
}

func (r *RedisStorage) Save(snapshot *snapshot.Snapshot) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.client.Set(ctx, r.key, data, 0).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}

	return nil
}

func (r *RedisStorage) Load() (*snapshot.Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := r.client.Get(ctx, r.key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var snap snapshot.Snapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w", err)
	}

	return &snap, nil
}

func (r *RedisStorage) Close() error {
	return r.client.Close()
}

