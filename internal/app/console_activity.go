package app

import (
	"github.com/erranliu/SteerLine/internal/console/activityrender"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
)

type consoleActivityFormatter struct{}

func (consoleActivityFormatter) FormatActivity(event executionmodel.ActivityEvent, color bool) string {
	return activityrender.RenderActivity(productquery.ActivityDisplayEvent(event), activityrender.RenderOptions{Color: color})
}
