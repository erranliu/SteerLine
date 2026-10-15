package testenv

import (
	"github.com/google/uuid"
	"strings"
)

func requestID() string { return strings.ReplaceAll(uuid.NewString(), "-", "") }
