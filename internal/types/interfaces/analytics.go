package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type DocumentAccessLogRepository interface {
	BulkCreate(ctx context.Context, logs []*types.DocumentAccessLog) error
}
