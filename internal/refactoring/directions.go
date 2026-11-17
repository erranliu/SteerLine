package refactoring

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/refactoring/model"
)

var ErrDirectionChanged = errors.New("重构方向已更新，请重新选择并确认正文")

func DefaultDirection() (model.Direction, error) {
	raw, err := prompts.MaintenanceDirection()
	if err != nil {
		return model.Direction{}, err
	}
	return model.Direction{
		ID: model.DefaultDirectionID, Name: model.DefaultDirectionName,
		Prompt: string(raw), Revision: model.Digest(raw),
	}, nil
}

func ResolveDirection(ctx context.Context, in model.Start, read func(context.Context, string) (model.Direction, error)) (model.Direction, error) {
	var direction model.Direction
	var err error
	if in.Direction == model.DefaultDirectionID {
		direction, err = DefaultDirection()
	} else {
		direction, err = read(ctx, in.Direction)
	}
	if err != nil {
		return model.Direction{}, err
	}
	if direction.Revision != in.DirectionRevision {
		return model.Direction{}, ErrDirectionChanged
	}
	direction.Prompt = in.DirectionPrompt
	return direction, nil
}
