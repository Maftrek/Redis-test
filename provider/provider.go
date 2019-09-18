package provider

import (
	"fmt"
	"redis-test/models"
	"time"

	"github.com/adjust/rmq"
	"github.com/go-redis/redis"
)

// Provider interface - для создания подключений и использования их в остальнои проекте

type Provider interface {
	GetConnectRedis() *redis.Client
	GetConnectRMQ() rmq.Connection
}

type provider struct {
	redisClient *redis.Client
	rmqRedis    rmq.Connection
}

func New(redisConfig models.Redis) (Provider, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         redisConfig.Host,
		Password:     redisConfig.Password,
		DB:           1,
		DialTimeout:  time.Duration(redisConfig.DialTimeout),
		ReadTimeout:  time.Duration(redisConfig.ReadTimeout),
		WriteTimeout: time.Duration(redisConfig.WriteTimeout),
	})

	_, err := client.Ping().Result()
	if err != nil {
		client.Options().Addr = "redis:6379"
		_, err = client.Ping().Result()
		if err != nil {
			err = fmt.Errorf("can't connect: %v.\nMake sure a redis is running at: %s", err, redisConfig.Host)
			return nil, err
		}
	}

	connection := rmq.OpenConnectionWithRedisClient("generate", client)

	return &provider{
		redisClient: client,
		rmqRedis:    connection,
	}, nil
}

func (p *provider) GetConnectRedis() *redis.Client {
	return p.redisClient
}

func (p *provider) GetConnectRMQ() rmq.Connection {
	return p.rmqRedis
}
