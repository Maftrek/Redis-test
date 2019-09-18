package service

import (
	"redis-test/repository"
	"time"

	"github.com/go-kit/kit/log"
)

// Service interface
type Service interface {
	Generator(errorsListen chan error)
	Subscriber(errorsListen chan error)
	IsMaster() (bool, error)
	SettingMaster() error
	Close()
	GetErrors() []string
}

type service struct {
	repository repository.Repository
	logger     log.Logger
}

// New return new service
func New(rep repository.Repository, logger log.Logger) Service {
	return &service{
		repository: rep,
		logger:     logger,
	}
}

func (s *service) Close() {
	if !s.repository.IsWasConsumer() {
		s.repository.StopConsumer()
	}
}

// генератор
func (s *service) Generator(errorsListen chan error) {
	for {
		// дополнительная проверка
		ok, err := s.repository.IsGenerator()
		if err != nil {
			s.logger.Log("err", err)
			continue
		}
		if !ok {
			s.logger.Log("master", "changed state to the subscriber")
			go s.Subscriber(errorsListen)
			return
		}

		err = s.repository.GeneratorAction()
		if err != nil {
			errorsListen <- err
		}
	}
}

// слушатель
func (s *service) Subscriber(errorsListen chan error) {
	// слушает очередь
	go s.repository.StartConsumer()

	// обрабатывает данные из очереди
	go s.repository.HandlerMsg()

	// проверка жив ли мастер
	s.CheckAliveMaster(errorsListen)
}

func (s *service) IsMaster() (bool, error) {
	return s.repository.IsGenerator()
}

func (s *service) SettingMaster() error {
	err := s.repository.SetMaster()
	if err != nil {
		return err
	}

	s.logger.Log("action", "applying settings...")

	return nil
}

func (s *service) CheckAliveMaster(errorsListen chan error) {
	finishCheck := make(chan bool, 1)
	// проверка не стал ли слушатель генератором
	go func() {
		for range time.NewTicker(time.Millisecond * 700).C {
			ok, err := s.repository.IsGenerator()
			if err != nil {
				s.logger.Log("err", err)
				continue
			}
			//  если слушатель стал генератором, то остановка подписки и запуск генератора
			if ok {
				select {
				case finishCheck <- true:
				default:
				}
				s.logger.Log("subscriber", "changed state to the master", time.Now())
				s.repository.StopConsumer()
				go s.Generator(errorsListen)
				return
			}
		}
	}()

	// проверка времени отправки последнего сообщения от мастера
	for range time.NewTicker(time.Millisecond * 700).C {
		select {
		case <-finishCheck:
			break
		default:
		}

		// если отправки не было больше секунды, то слушатель начинает настройку под мастера
		if s.repository.IsExpireMsg() {
			// удаление старого мастера
			countDel := s.repository.DeleteOldMaster()
			s.logger.Log("delete of a broken master from queue", countDel)

			err := s.SettingMaster()
			if err != nil {
				errorsListen <- err
				s.logger.Log("err", err)
			}
		}
	}
}

func (s *service) GetErrors() []string {
	return s.repository.GetQueueErrors()
}
