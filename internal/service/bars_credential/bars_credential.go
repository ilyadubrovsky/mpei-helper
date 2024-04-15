package bars_credential

import (
	"context"
	"fmt"
	"sync"

	"github.com/ilyadubrovsky/tracking-bars/internal/config"
	"github.com/ilyadubrovsky/tracking-bars/internal/domain"
	ierrors "github.com/ilyadubrovsky/tracking-bars/internal/errors"
	"github.com/ilyadubrovsky/tracking-bars/internal/repository"
	"github.com/ilyadubrovsky/tracking-bars/internal/service"
	"github.com/ilyadubrovsky/tracking-bars/pkg/aes"
	"github.com/ilyadubrovsky/tracking-bars/pkg/bars"
)

// TODO здесь должнен быть пул клиентов, реализация с мьютексом медленная
// нужно сбрасывать клиента через Clear() после использования перед возвращением в пул
type svc struct {
	userSvc             service.User
	barsCredentialsRepo repository.BarsCredentials
	cfg                 config.Bars

	mu         sync.Mutex
	barsClient bars.Client
}

func NewService(
	userSvc service.User,
	barsCredentialsRepo repository.BarsCredentials,
	cfg config.Bars,
) *svc {
	return &svc{
		userSvc:             userSvc,
		barsCredentialsRepo: barsCredentialsRepo,
		cfg:                 cfg,
		barsClient:          bars.NewClient(config.BARSRegistrationPageURL),
	}
}

func (s *svc) Authorization(ctx context.Context, credentials *domain.BarsCredentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.barsClient.Clear(ctx)

	repoCredentials, err := s.barsCredentialsRepo.Get(ctx, credentials.UserID)
	if err != nil {
		return fmt.Errorf("barsCredentialsRepo.Get: %w", err)
	}
	if repoCredentials != nil {
		return ierrors.ErrAlreadyAuth
	}

	err = s.barsClient.Authorization(ctx, credentials.Username, string(credentials.Password))
	if err != nil {
		return fmt.Errorf("barsClient.Authorization: %w", err)
	}

	encryptedPassword, err := aes.Encrypt([]byte(s.cfg.EncryptionKey), credentials.Password)
	if err != nil {
		return fmt.Errorf("aes.Encrypt (password): %w", err)
	}
	credentials.Password = encryptedPassword

	err = s.barsCredentialsRepo.Save(ctx, credentials)
	if err != nil {
		return fmt.Errorf("barsCredentialsRepo.Save: %w", err)
	}

	return nil
}

func (s *svc) Logout(ctx context.Context, userID int64) error {
	err := s.barsCredentialsRepo.Delete(ctx, userID)
	if err != nil {
		return fmt.Errorf("barsCredentialsRepo.Delete: %w", err)
	}

	return nil
}