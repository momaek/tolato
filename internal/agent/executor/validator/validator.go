package validator

import (
	"errors"

	"github.com/momaek/tolato/internal/agent/executor/runner"
	"github.com/momaek/tolato/internal/shared/action"
)

type Validator interface {
	Validate(job runner.Job) error
}

type RegistryValidator struct{}

func NewRegistryValidator() RegistryValidator {
	return RegistryValidator{}
}

func (RegistryValidator) Validate(job runner.Job) error {
	if len(job.Steps) == 0 {
		return errors.New("job has no steps")
	}

	for _, step := range job.Steps {
		if _, ok := action.Get(step.Action); !ok {
			return errors.New("step action is not registered")
		}
	}

	return nil
}
