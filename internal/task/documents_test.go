package task

import (
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"testing"
)

func TestAcceptanceHeadingRecognizesChineseAndEnglish(t *testing.T) {
	for _, content := range []string{"## 9. 验收\n", "### Acceptance Criteria\n"} {
		if !taskmodel.HasAcceptanceHeading([]byte(content)) {
			t.Fatalf("acceptance heading was not recognized: %q", content)
		}
	}
	for _, content := range []string{"验收要求见下文\n", "## 设计\n"} {
		if taskmodel.HasAcceptanceHeading([]byte(content)) {
			t.Fatalf("non-acceptance heading was recognized: %q", content)
		}
	}
}
