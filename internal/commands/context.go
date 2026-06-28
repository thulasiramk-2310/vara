package commands

import (
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/logging"
)

// Context encapsulates the state required for a CLI command to execute.
type Context struct {
	Repository *repository.Repository
	Index      *index.Index
	Logger     logging.Logger
}
