package surface

import (
	"context"

	"github.com/0venburn/daemon/internal/protocol"
)

type Surface interface {
	Read(ctx context.Context, path string) (string, error)
	Edit(ctx context.Context, op protocol.EditOp) (protocol.EditResult, error)
	Publish(ctx context.Context, event protocol.Event) error
}
