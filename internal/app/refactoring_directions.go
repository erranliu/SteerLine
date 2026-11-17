package app

import (
	"context"

	"github.com/erranliu/SteerLine/internal/refactoring"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func (d *ApplicationDirectory) RefactoringDirections(ctx context.Context) (ref.DirectionCatalogue, error) {
	var catalogue ref.DirectionCatalogue
	builtIn, err := refactoring.DefaultDirection()
	if err != nil {
		return catalogue, err
	}
	catalogue.Directions = []ref.Direction{builtIn}
	items, err := (sqlite.RefactoringDirections{Home: d.home}).List(ctx)
	if err != nil {
		catalogue.Diagnostics = []string{"自定义重构方向暂不可用：" + err.Error()}
	} else {
		catalogue.Directions = append(catalogue.Directions, items...)
	}
	return catalogue, nil
}

func (d *ApplicationDirectory) ChangeRefactoringDirection(ctx context.Context, change ref.DirectionChange) (ref.Direction, error) {
	return (sqlite.RefactoringDirections{Home: d.home}).Change(ctx, change)
}
