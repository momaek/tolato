package actions

import (
	"github.com/momaek/tolato/internal/shared/action"
	"github.com/momaek/tolato/internal/shared/types"
)

func List() []types.ActionSpec {
	return action.List()
}
