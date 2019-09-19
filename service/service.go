package service

import (
	"errors"
	"time"

	"github.com/Maftrek/redis-test/repository"

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

	counterWithoutMaster := 0
	changer := s.repository.GetChanger()

	// проверка времени отправки последнего сообщения от мастера
	for range time.NewTicker(time.Second).C {
		select {
		case <-finishCheck:
			break
		default:
		}

		// если сменщик мастера не успевает сделать настройку за две итерации без мастера, то сменшик заменяется
		s.changeChanger(&counterWithoutMaster, changer)

		changer = s.repository.GetChanger()

		// проверка не было ли отправки больше секунды назад
		if s.repository.IsExpireMsg() {
			err := s.actionExpireMsg(&counterWithoutMaster, changer)
			if err != nil {
				errorsListen <- err
			}
		} else {
			counterWithoutMaster = 0
		}
	}
}

func (s *service) GetErrors() []string {
	return s.repository.GetQueueErrors()
}

func (s *service) actionExpireMsg(counterWithoutMaster *int, changer int64) error {
	if counterWithoutMaster == nil {
		return errors.New("nil counter")
	}
	// слушатель проверяет является ли он сменщиком мастера
	// если да, то начинает настройку под мастера
	if s.repository.IsChanger() {
		// удаление старого мастера
		countDel := s.repository.DeleteOldMaster()
		s.logger.Log("delete of a broken master from queue", countDel)

		err := s.SettingMaster()
		if err != nil {
			s.logger.Log("err", err)
			return err
		}
		return nil
	}

	// если нет, то начинает считать сколько итераций мастера нет
	if changer == s.repository.GetChanger() {
		s.logger.Log("changer not response", changer)
		*counterWithoutMaster++
	}

	return nil
}

// если сменщик мастера не успевает сделать настройку за две итерации без мастера, то сменшик заменяется
func (s *service) changeChanger(counterWithoutMaster *int, changer int64) {
	if counterWithoutMaster == nil {
		return
	}

	if *counterWithoutMaster >= 2 && changer == s.repository.GetChanger() {
		s.logger.Log("delete changer", changer)
		s.repository.DeleteChanger(changer)
		*counterWithoutMaster = 0
	}
}
