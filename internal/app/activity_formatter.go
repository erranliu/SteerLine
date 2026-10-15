package app

import (
	"github.com/erranliu/SteerLine/internal/product/activityrender"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
)

type serviceActivityFormatter struct{}

func (serviceActivityFormatter) FormatActivity(event executionmodel.ActivityEvent, color bool) string {
	return activityrender.RenderActivity(productquery.ActivityDisplayEvent(event), activityrender.RenderOptions{Color: color})
}
