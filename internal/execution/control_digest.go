package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

func RequestDigest(req productapi.ControlRequest) string {
	b, _ := json.Marshal(req)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
