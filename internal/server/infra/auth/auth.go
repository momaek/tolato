package auth

import (
	"errors"

	"github.com/momaek/tolato/internal/shared/types"
)

type Service struct {
	adminUsername string
	adminPassword string
}

func NewService(username, password string) Service {
	return Service{
		adminUsername: username,
		adminPassword: password,
	}
}

func (s Service) Login(username, password string) (types.CurrentUser, error) {
	if username != s.adminUsername || password != s.adminPassword {
		return types.CurrentUser{}, errors.New("invalid credentials")
	}

	return s.CurrentUser(), nil
}

func (s Service) CurrentUser() types.CurrentUser {
	return types.CurrentUser{
		ID:   "u_admin",
		Role: "admin",
	}
}
