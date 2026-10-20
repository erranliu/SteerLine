package runnercontract

import (
	"io/fs"
	"strings"
	"time"
	"unicode"
)

const ArtifactDirectory = ".steerrunner/artifacts"
const MaxArtifactBytes = 1 << 20
const MaxRunArtifactBytes = 16 << 20
const MaxRunArtifacts = 64
const MaxDiscoveryEntries = 256

type CollectArtifactInput struct {
	ExpectedRevision uint64 `json:"expected_revision"`
	RelativePath     string `json:"relative_path"`
}

// Paths are portable, slash-separated names relative to ArtifactDirectory.
func ValidArtifactPath(path string) bool {
	if !fs.ValidPath(path) || path == "." || len(path) > 1024 || strings.ContainsAny(path, "\\:") || strings.IndexFunc(path, unicode.IsControl) >= 0 {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
	}
	return true
}

type Artifact struct {
	RunID        string    `json:"run_id"`
	ArtifactID   string    `json:"artifact_id"`
	RelativePath string    `json:"relative_path"`
	SHA256       string    `json:"sha256"`
	SizeBytes    int       `json:"size_bytes"`
	CollectedAt  time.Time `json:"collected_at"`
	ReadCommand  string    `json:"read_command"`
}
type ArtifactContent struct {
	Artifact
	Encoding      string `json:"encoding"`
	ContentBase64 string `json:"content_base64"`
}
type ArtifactCandidate struct {
	RelativePath string `json:"relative_path"`
	SizeBytes    int64  `json:"size_bytes"`
}
type ArtifactDiscovery struct {
	RunID      string              `json:"run_id"`
	Directory  string              `json:"directory"`
	Candidates []ArtifactCandidate `json:"candidates"`
}
type ArtifactList struct {
	RunID     string     `json:"run_id"`
	Artifacts []Artifact `json:"artifacts"`
}
type WorkspaceGit struct {
	RunID      string    `json:"run_id"`
	Commit     *string   `json:"commit"`
	Clean      *bool     `json:"clean"`
	ObservedAt time.Time `json:"observed_at"`
	Verified   bool      `json:"verified"`
	Diagnostic string    `json:"diagnostic"`
}
