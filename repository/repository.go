package repository

import (
	"errors"
	"fmt"
	"math/rand"
	"redis-test/provider"
	"time"

	"github.com/adjust/rmq"
	"github.com/go-kit/kit/log"
	"github.com/go-redis/redis"
	"github.com/pborman/uuid"
)

const QueueName = "sub1"
const QueueErrorsName = "errors"
const singSend = "send"
const layout = "2006-01-02T15:04:05.999999999Z07:00"

// Repository interface
type Repository interface {
	SetMaster() error
	IsGenerator() (bool, error)
	GeneratorAction() error
	StartConsumer()
	HandlerMsg()
	StopConsumer()
	DeleteOldMaster() int64
	GetQueueErrors() []string
	IsExpireMsg() bool
	IsWasConsumer() bool

	msgContainsError(data []byte)
	randomData() []byte
}

type repository struct {
	provider      provider.Provider
	lenRandomData int
	clientID      int64
	data          chan []byte
	chance        [20]bool
	taskQueue     rmq.Queue
	logger        log.Logger
}

// New repository
func New(pr provider.Provider, logger log.Logger) (Repository, error) {
	client := pr.GetConnectRedis()
	clientID, err := client.ClientID().Result()
	if err != nil {
		return nil, err
	}

	return &repository{
		provider:      pr,
		lenRandomData: 5,
		clientID:      clientID,
		data:          make(chan []byte),
		logger:        logger,
		chance:        [20]bool{true},
	}, nil
}

func (r *repository) randomData() []byte {
	var letter = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]byte, r.lenRandomData)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return b
}

func (r *repository) SetMaster() error {
	client := r.provider.GetConnectRedis()

	_, err := client.SetNX("master", r.clientID, 0).Result()
	if err != nil {
		r.logger.Log("err", err)
		return err
	}

	return nil
}

func (r *repository) IsGenerator() (bool, error) {
	client := r.provider.GetConnectRedis()
	res, err := client.Get("master").Int()
	if err != nil {
		r.logger.Log("err", err)
		return false, err
	}
	if int64(res) != r.clientID {
		return false, nil
	}

	return true, nil
}

func (r *repository) GeneratorAction() error {
	// публикация раз в 500 милисекунд
	time.Sleep(time.Millisecond * 500)

	data := r.randomData()

	connect := r.provider.GetConnectRMQ()
	taskQueue := connect.OpenQueue(QueueName)
	ok := taskQueue.PublishBytes(data)
	if !ok {
		err := errors.New("publish is not ok")
		r.logger.Log("err", err)
		return err
	}

	// обновление времени последней отправки
	client := r.provider.GetConnectRedis()
	timeNow := time.Now().Format(layout)
	_, err := client.Set(singSend, timeNow, time.Second).Result()
	if err != nil {
		r.logger.Log("err", err)
		return err
	}

	r.logger.Log("send", string(data))

	return nil
}

type Consumer struct {
	data chan []byte
	tag  string
}

func (r *repository) StartConsumer() {
	connect := r.provider.GetConnectRMQ()
	r.taskQueue = connect.OpenQueue(QueueName)
	ok := r.taskQueue.StartConsuming(10, time.Millisecond)
	if !ok {
		r.logger.Log("start consuming", "fail")
		return
	}
	consumer := &Consumer{
		data: r.data,
		tag:  uuid.New(),
	}

	r.taskQueue.AddConsumer(consumer.tag, consumer)

	r.logger.Log("start consuming", "success")

	go func() {
		for range time.Tick(time.Minute) {
			r.taskQueue.ReturnAllRejected()
		}
	}()
}

func (r *repository) IsWasConsumer() bool {
	return r.taskQueue != nil
}

func (r *repository) StopConsumer() {
	if r.taskQueue == nil {
		r.logger.Log("taskQueue", "empty")
		return
	}

	dataFinished := r.taskQueue.StopConsuming()

	for data := range dataFinished {
		r.data <- []byte(fmt.Sprintf("%v", data))
	}

	r.logger.Log("stop consuming", "success")
}

func (consumer *Consumer) Consume(delivery rmq.Delivery) {
	consumer.data <- []byte(delivery.Payload())
	delivery.Ack()
}

func (r *repository) HandlerMsg() {
	for dataMsg := range r.data {
		r.logger.Log("get msg", string(dataMsg))
		r.msgContainsError(dataMsg)
	}
}

func (r *repository) GetQueueErrors() []string {
	client := r.provider.GetConnectRedis()
	errorsList := client.LRange(QueueErrorsName, 0, -1).Val()
	count := client.Del(QueueErrorsName)
	r.logger.Log("delete count errors", count)
	return errorsList
}

func (r *repository) DeleteOldMaster() int64 {
	client := r.provider.GetConnectRedis()
	count := client.Del("master")

	timeNow := time.Now().Format(layout)

	_, err := client.Set(singSend, timeNow, time.Second).Result()
	if err != nil {
		r.logger.Log("err", err)
		return 0
	}

	return count.Val()
}

func (r *repository) msgContainsError(data []byte) {
	rand.Seed(time.Now().UnixNano())
	index := rand.Intn(len(r.chance) - 1)
	if r.chance[index] {
		client := r.provider.GetConnectRedis()
		id := client.LPush(QueueErrorsName, string(data))
		r.logger.Log("errors add", id)
	}
}

func (r *repository) IsExpireMsg() bool {
	client := r.provider.GetConnectRedis()
	res, err := client.Get(singSend).Result()

	if err != nil && err != redis.Nil {
		r.logger.Log("err", err)
		return false
	}
	timeGet := time.Time{}
	if err != redis.Nil {
		timeGet, err = time.Parse(layout, res)
		if err != nil {
			r.logger.Log("err", err)
			return false
		}
	}
	timeMatch := timeGet.Add(time.Second)
	return timeMatch.Before(time.Now())
}
