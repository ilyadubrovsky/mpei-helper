package service

import (
	"errors"
	"fmt"

	"github.com/ilyadubrovsky/tracking-bars/internal/config"
	"github.com/ilyadubrovsky/tracking-bars/internal/service"
	tele "gopkg.in/telebot.v3"
	"gopkg.in/telebot.v3/middleware"
)

type svc struct {
	bot         *tele.Bot
	cfg         *config.Telegram
	userService service.User
	barsService service.Bars
}

func NewTelegram(
	cfg *config.Telegram,
	userService service.User,
	barsService service.Bars,
) (*svc, error) {
	bot, err := createBot(cfg)
	if err != nil {
		return nil, fmt.Errorf("createBot: %w", err)
	}

	s := &svc{
		bot:         bot,
		cfg:         cfg,
		userService: userService,
		barsService: barsService,
	}

	s.setBotSettings()

	return s, nil
}

func createBot(cfg *config.Telegram) (*tele.Bot, error) {
	pref := tele.Settings{
		Token:  cfg.BotToken,
		Poller: &tele.LongPoller{Timeout: cfg.LongPollerDelay},
	}

	abot, err := tele.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("tele.NewBot: %w", err)
	}

	return abot, nil
}

func (s *svc) setBotSettings() {
	// TODO провалидировать всю логику заново так как телебот обновился
	s.bot.Handle(tele.OnCallback, s.handleCallback)

	s.bot.Handle("/start", s.handleStartCommand)

	s.bot.Handle("/help", s.handleHelpCommand)

	s.bot.Handle("/fixgrades", s.handleFixGradesCommand)

	s.bot.Handle("/auth", s.handleAuthCommand)

	s.bot.Handle("/logout", s.handleLogoutCommand)

	s.bot.Handle("/pt", s.handleProgressTableCommand)

	s.bot.Handle("/gh", s.handleGithubCommand)

	s.bot.Handle(tele.OnText, s.handleText)

	adminGroup := s.bot.Group()
	adminGroup.Use(
		middleware.Whitelist(
			s.cfg.AdminID,
		),
	)

	adminGroup.Handle("/aecho", s.handleAdminEchoCommand)

	adminGroup.Handle("/asm", s.handleAdminSendMessageCommand)
}

func (s *svc) SendMessageWithOpts(id int64, message string, opts ...interface{}) error {
	chat := tele.ChatID(id)

	_, err := s.bot.Send(chat, message, opts...)

	return s.middlewareError(id, err)
}

func (s *svc) EditMessageWithOpts(id int64, messageID int, msg string, opts ...interface{}) error {
	_, err := s.bot.Edit(
		&editableMessage{
			messageID: messageID,
			chatID:    id,
		},
		msg,
		opts...,
	)

	if errors.Is(err, tele.ErrTrueResult) {
		err = s.SendMessageWithOpts(id, "TODO bot error")
	}

	return s.middlewareError(id, err)
}

// TODO в теории тут можно обрабатывать и все сервисные ошибки
func (s *svc) middlewareError(targetUserID int64, err error) error {
	if err == nil {
		return nil
	}

	if errors.As(err, &tele.ErrBlockedByUser) ||
		errors.As(err, &tele.ErrUserIsDeactivated) ||
		errors.As(err, &tele.ErrNotStartedByUser) {
		// TODO удаление пользователя, чтобы больше не пытаться ему отправить что-либо
	}

	return err
}
