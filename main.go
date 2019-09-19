package main

import (
	"os"
	"github.com/Maftrek/redis-test/models"

	"github.com/Maftrek/redis-test/application"
	"github.com/Maftrek/redis-test/provider"
	"github.com/Maftrek/redis-test/repository"
	"github.com/Maftrek/redis-test/service"

	"github.com/go-kit/kit/log"
)

func initialization() (models.Config, log.Logger) {
	// инициализация логгера (запись в формате json, указание caller)
	logger := log.With(
		log.NewJSONLogger(os.Stderr),
		"caller", log.DefaultCaller,
	)
	// чтения конфига из файла
	var appConfig models.Config
	err := models.LoadConfig(&appConfig)
	if err != nil {
		logger.Log("err", err)
		os.Exit(1)
	}
	return appConfig, logger
}

func main() {
	appConfig, logger := initialization()
	// настройка redis и rmq
	pr, err := provider.New(appConfig.RedisConfig)
	if err != nil {
		logger.Log("err", err)
		os.Exit(1)
	}

	// инициализация репозитория для работы с redis
	rep, err := repository.New(pr, logger)

	if err != nil {
		logger.Log("err", err)
		os.Exit(1)
	}

	// инициализация сервиса
	svc := service.New(rep, logger)

	// создание application, где осуществляются вызовы функций сервиса для работы программы
	app := application.New(&application.Options{
		Svc:    svc,
		Logger: logger,
	})

	// определение с какими аргументами запущено приложение и соответствующие действия
	for _, arg := range os.Args {
		if arg == "getErrors" {
			app.GetErrors()
			return
		}
	}

	app.Start()
}
