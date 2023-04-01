package app

import (
	"context"
	"fmt"
	"github.com/streadway/amqp"
	"os"
	"sync"
	"user-service/internal/config"
	"user-service/internal/entity/user"
	"user-service/internal/events"
	"user-service/internal/events/authorization"
	"user-service/internal/events/deleteuser"
	"user-service/internal/events/logout"
	"user-service/internal/events/news"
	"user-service/internal/service"
	storage "user-service/internal/storage/postgresql"
	"user-service/pkg/client/mq"
	"user-service/pkg/client/mq/rabbitmq"
	"user-service/pkg/client/postgresql"
	"user-service/pkg/logging"
)

type App struct {
	service            Service
	usersStorage       user.Repository
	cfg                *config.Config
	logger             *logging.Logger
	producer           mq.Producer
	authStrategy       events.ProcessStrategy
	logoutStrategy     events.ProcessStrategy
	newsStrategy       events.ProcessStrategy
	deleteUserStrategy events.ProcessStrategy
}

type Service interface {
	Authorization(ctx context.Context, dto user.CreateUserDTO) (bool, error)
	Logout(ctx context.Context, id int64) error
	GetUsersIDByOpts(ctx context.Context, opts ...string) ([]user.User, error)
	DeleteUser(ctx context.Context, id int64) error
}

func NewApp(cfg *config.Config) (*App, error) {
	var a App
	a.cfg = cfg

	a.logger = logging.GetLogger()

	pgConfig := postgresql.NewPgConfig(os.Getenv("PG_USERNAME"), os.Getenv("PG_PASSWORD"),
		os.Getenv("PG_HOST"), os.Getenv("PG_PORT"), os.Getenv("PG_DATABASE"))

	a.logger.Info("postgresql client initializing")
	postgresqlClient, err := postgresql.NewClient(context.Background(), pgConfig)
	if err != nil {
		a.logger.Errorf("failed to connection to postgresql due to error: %v", err)
		return nil, err
	}

	a.logger.Info("users storage initializing")
	a.usersStorage = storage.NewUsersPostgres(postgresqlClient, a.logger)

	a.logger.Info("service initializing")
	a.service = service.NewService(cfg, a.usersStorage, a.logger)

	// TODO responses instead config
	a.logger.Info("auth process strategy initializing")
	a.authStrategy = authorization.NewProcessStrategy(a.service, a.cfg)

	// TODO responses instead config
	a.logger.Info("logout process strategy initializing")
	a.logoutStrategy = logout.NewProcessStrategy(a.service, a.cfg)

	a.logger.Info("news process strategy initializing")
	a.newsStrategy = news.NewProcessStrategy(a.service, a.cfg.Responses.BotError)

	a.logger.Info("deleteuser process strategy initializing")
	a.deleteUserStrategy = deleteuser.NewProcessStrategy(a.service)

	RabbitURL := fmt.Sprintf("amqp://%s:%s@%s:%s",
		os.Getenv("RABBIT_USERNAME"), os.Getenv("RABBIT_PASSWORD"),
		os.Getenv("RABBIT_HOST"), os.Getenv("RABBIT_PORT"))

	a.logger.Info("rabbitmq producer initializing")
	producer, err := rabbitmq.NewProducer(RabbitURL)
	if err != nil {
		a.logger.Fatalf("failed to create a new producer due to error: %v", err)
	}

	a.logger.Info("producer: 'telegram' exchange initializing")
	if err = producer.DeclareExchange(a.cfg.RabbitMQ.Producer.TelegramExchange, amqp.ExchangeDirect,
		true, false, false); err != nil {
		a.logger.Fatalf("failed to declare an exchange due to error: %v", err)
	}

	a.logger.Info("producer: 'telegram messages' queue initializing and binding")
	if err = producer.DeclareAndBindQueue(a.cfg.RabbitMQ.Producer.TelegramExchange,
		a.cfg.RabbitMQ.Producer.TelegramMessages, a.cfg.RabbitMQ.Producer.TelegramMessagesKey); err != nil {
		a.logger.Fatalf("failed to declare and bind a queue due to error: %v", err)
	}

	a.producer = producer
	return &a, nil
}

func (a *App) startConsume() {
	a.logger.Info("start consume")

	RabbitURL := fmt.Sprintf("amqp://%s:%s@%s:%s/",
		os.Getenv("RABBIT_USERNAME"), os.Getenv("RABBIT_PASSWORD"),
		os.Getenv("RABBIT_HOST"), os.Getenv("RABBIT_PORT"))

	a.logger.Info("rabbitmq consumer initializing")
	consumer, err := rabbitmq.NewConsumer(RabbitURL, a.cfg.RabbitMQ.Consumer.PrefetchCount)
	if err != nil {
		a.logger.Fatalf("failed to create a consumer due to error: %v", err)
	}

	if err = a.initializeConsume(consumer, a.cfg.RabbitMQ.Consumer.AuthRequests, a.cfg.RabbitMQ.Producer.TelegramExchange,
		a.cfg.RabbitMQ.Producer.TelegramMessagesKey, a.cfg.RabbitMQ.Consumer.AuthWorkers, a.authStrategy); err != nil {
		a.logger.Fatalf("failed to initialize consume due to error: %v", err)
	}

	if err = a.initializeConsume(consumer, a.cfg.RabbitMQ.Consumer.LogoutRequests, a.cfg.RabbitMQ.Producer.TelegramExchange,
		a.cfg.RabbitMQ.Producer.TelegramMessagesKey, a.cfg.RabbitMQ.Consumer.LogoutWorkers, a.logoutStrategy); err != nil {
		a.logger.Fatalf("failed to initialize consume due to error: %v", err)
	}

	if err = a.initializeConsume(consumer, a.cfg.RabbitMQ.Consumer.NewsRequests, a.cfg.RabbitMQ.Producer.TelegramExchange,
		a.cfg.RabbitMQ.Producer.TelegramMessagesKey, a.cfg.RabbitMQ.Consumer.NewsWorkers, a.newsStrategy); err != nil {
		a.logger.Fatalf("failed to initialize consume due to error: %v", err)
	}

	if err = a.initializeConsume(consumer, a.cfg.RabbitMQ.Consumer.DeleteUserRequests, a.cfg.RabbitMQ.Producer.TelegramExchange,
		a.cfg.RabbitMQ.Producer.TelegramMessagesKey, a.cfg.RabbitMQ.Consumer.DeleteUserWorkers, a.deleteUserStrategy); err != nil {
		a.logger.Fatalf("failed to initialize consume due to error: %v", err)
	}
}

func (a *App) initializeConsume(consumer mq.Consumer, queue, exchange, key string, workers int, strategy events.ProcessStrategy) error {
	a.logger.Infof("consumer: '%s' queue initializing", queue)
	if err := consumer.DeclareQueue(queue, true, false, false); err != nil {
		return fmt.Errorf("declare queue: %v", err)
	}

	messages, err := consumer.Consume(queue, false, false)
	if err != nil {
		return fmt.Errorf("consume: %v", err)
	}

	for i := 0; i < workers; i++ {
		worker := events.NewWorker(a.logger, consumer, a.producer, exchange, key, strategy, messages)
		go worker.Process()
		a.logger.Infof("queue '%s' worker [%d] started", queue, i+1)
	}

	return nil
}

func (a *App) Run() {
	a.logger.Info("app launching")

	var wg sync.WaitGroup
	wg.Add(1)

	a.startConsume()

	wg.Wait()
}
