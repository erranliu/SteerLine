package execution

import (
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
)

type centerReader interface{ persistence.CenterReader }
