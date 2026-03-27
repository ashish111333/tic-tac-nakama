package main

import (
	"context"
	"database/sql"

	infra "tic-tac-nakama/internal/infrastructure/nakama"

	"github.com/heroiclabs/nakama-common/runtime"
)

func InitModule(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {
	module := infra.NewModule()
	return module.Register(ctx, logger, db, nk, initializer)
}
