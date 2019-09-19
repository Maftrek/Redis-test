package application

import (
	"os"
	"os/signal"
	"github.com/Maftrek/redis-test/service"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
)

// Application struct
type Application struct {
	svc    service.Service
	logger log.Logger
}

// Options struct
type Options struct {
	Svc    service.Service
	Logger log.Logger
}

// New return new application
func New(opt *Options) *Application {
	return &Application{
		svc:    opt.Svc,
		logger: opt.Logger,
	}
}

// Start application
func (app *Application) Start() {
	app.logger.Log("server", "start")

	//  настройка мастера
	err := app.svc.SettingMaster()
	if err != nil {
		app.logger.Log("err", err)
	}

	// проверка является ли данное приложение мастером
	isMaster, err := app.svc.IsMaster()
	if err != nil {
		app.logger.Log("err", err)
	}

	// канал ошибок
	errorsListen := make(chan error, 1)

	// начало работы в зависимости от типа приложения (мастер / слушатель)
	if isMaster {
		go app.svc.Generator(errorsListen)
	} else {
		go app.svc.Subscriber(errorsListen)
	}

	// слушатель ошибок
	go func() {
		for range time.Tick(time.Second) {
			err := <-errorsListen
			if err != nil {
				app.logger.Log("err", err)
			}
		}
	}()

	// ожидание сигнала от системы
	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, syscall.SIGINT, syscall.SIGTERM)

	<-osSignals

	app.logger.Log("exit", "active")
	app.svc.Close()
	os.Exit(1)
}

// возврат ошибок из очереди
func (app *Application) GetErrors() {
	res := app.svc.GetErrors()
	for _, errorItem := range res {
		app.logger.Log("error", errorItem)
	}
}
