package publication

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// ProviderRef freezes the non-secret provider used to verify real delivery.
type ProviderRef struct {
	ConfigDigest    string `json:"config_digest"`
	Kind            string `json:"kind"`
	BaseURL         string `json:"base_url"`
	Repository      string `json:"repository"`
	QueryExecutable string `json:"query_executable"`
	QueryArgsDigest string `json:"query_args_digest"`
}

func ProviderIdentity(config Config) ProviderRef {
	full, _ := json.Marshal(config)
	fullDigest := sha256.Sum256(full)
	args, _ := json.Marshal(config.Query.Args)
	digest := sha256.Sum256(args)
	return ProviderRef{ConfigDigest: hex.EncodeToString(fullDigest[:]), Kind: config.Kind, BaseURL: config.BaseURL, Repository: config.Repository, QueryExecutable: config.Query.Executable, QueryArgsDigest: hex.EncodeToString(digest[:])}
}
func (identity ProviderRef) Matches(config Config) bool { return identity == ProviderIdentity(config) }
