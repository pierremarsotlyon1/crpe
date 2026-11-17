package objectstore

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/pierremarsotlyon1/crpe/backend/internal/worker"
)

// DeleteHandler also handles abandoned staged uploads. Object keys are unique
// and never reused; Delete is idempotent when the object is already absent.
func DeleteHandler(store Store) worker.Handler {
	return func(ctx context.Context, msg worker.Message) error {
		var in struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(msg.Payload, &in); err != nil || in.Key == "" {
			return errors.New("invalid object cleanup job")
		}
		if store == nil {
			return errors.New("object storage unavailable")
		}
		return store.Delete(ctx, in.Key)
	}
}
