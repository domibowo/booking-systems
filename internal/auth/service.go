package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailTaken         = errors.New("auth: email already registered")
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrInvalidToken       = errors.New("auth: invalid or expired refresh token")
)

// UserRepository and RefreshTokenRepository are defined here, on the
// consumer side, per the project's dependency-inversion convention.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *RefreshToken) error
	FindByHash(ctx context.Context, hash string) (*RefreshToken, error)
	Revoke(ctx context.Context, id uuid.UUID) error
}

type TokenConfig struct {
	Secret          []byte
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

type Service struct {
	users  UserRepository
	tokens RefreshTokenRepository
	cfg    TokenConfig
}

func NewService(users UserRepository, tokens RefreshTokenRepository, cfg TokenConfig) *Service {
	return &Service{users: users, tokens: tokens, cfg: cfg}
}

type AuthResult struct {
	User         *User
	AccessToken  string
	RefreshToken string
}

func (s *Service) Register(ctx context.Context, email, password, fullName string) (*AuthResult, error) {
	existing, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrEmailTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &User{
		Email:        email,
		PasswordHash: string(hash),
		FullName:     fullName,
		Role:         RoleCustomer,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	return s.issueTokens(ctx, user)
}

func (s *Service) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	user, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.issueTokens(ctx, user)
}

// Refresh rotates the refresh token: the old one is revoked and a new
// access/refresh pair is issued, so a stolen refresh token can only be used
// once before detection.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*AuthResult, error) {
	hash := hashRefreshToken(refreshToken)
	stored, err := s.tokens.FindByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if stored == nil || stored.RevokedAt != nil || stored.ExpiresAt.Before(time.Now()) {
		return nil, ErrInvalidToken
	}

	user, err := s.users.FindByID(ctx, stored.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidToken
	}

	if err := s.tokens.Revoke(ctx, stored.ID); err != nil {
		return nil, err
	}

	return s.issueTokens(ctx, user)
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	hash := hashRefreshToken(refreshToken)
	stored, err := s.tokens.FindByHash(ctx, hash)
	if err != nil {
		return err
	}
	if stored == nil {
		return nil
	}
	return s.tokens.Revoke(ctx, stored.ID)
}

func (s *Service) ParseAccessToken(tokenString string) (*AccessClaims, error) {
	return parseAccessToken(s.cfg.Secret, tokenString)
}

func (s *Service) issueTokens(ctx context.Context, user *User) (*AuthResult, error) {
	access, err := generateAccessToken(s.cfg.Secret, user.ID, user.Role, s.cfg.AccessTokenTTL)
	if err != nil {
		return nil, err
	}

	plainRefresh, hash, err := generateRefreshToken()
	if err != nil {
		return nil, err
	}

	rt := &RefreshToken{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(s.cfg.RefreshTokenTTL),
	}
	if err := s.tokens.Create(ctx, rt); err != nil {
		return nil, err
	}

	return &AuthResult{User: user, AccessToken: access, RefreshToken: plainRefresh}, nil
}
