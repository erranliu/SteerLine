package execution

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func decode(data []byte, v any) error { return contracts.DecodeJSON(data, v) }
