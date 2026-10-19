package app

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestHTTPConsumerBoundary(t *testing.T) {
	for _, owner := range []string{"../product/httpclient", "../product/httpcontract", "../product/httpfs", "../projectconnection", "../product/applicationclient", "../product/applicationcontract", "../localservice", "../../web/server", "../../cmd/steerline-web"} {
		output, err := exec.Command("go", "list", "-deps", "-json", owner).Output()
		if err != nil {
			t.Fatalf("%s: %v", owner, err)
		}
		decoder := json.NewDecoder(strings.NewReader(string(output)))
		count := 0
		for decoder.More() {
			var pkg struct{ ImportPath string }
			if err = decoder.Decode(&pkg); err != nil {
				t.Fatal(err)
			}
			count++
			name := pkg.ImportPath
			if name == "database/sql" || strings.HasPrefix(name, "modernc.org/sqlite") || strings.HasPrefix(name, "github.com/erranliu/SteerLine/internal/") && !(strings.HasPrefix(name, "github.com/erranliu/SteerLine/internal/product/api") || strings.HasPrefix(name, "github.com/erranliu/SteerLine/internal/product/httpclient") || strings.HasPrefix(name, "github.com/erranliu/SteerLine/internal/product/httpcontract") || strings.HasPrefix(name, "github.com/erranliu/SteerLine/internal/product/httpfs") || name == "github.com/erranliu/SteerLine/internal/projectconnection" || name == "github.com/erranliu/SteerLine/internal/pathguard" || name == "github.com/erranliu/SteerLine/internal/localservice" || name == "github.com/erranliu/SteerLine/internal/product/applicationclient" || name == "github.com/erranliu/SteerLine/internal/product/applicationcontract") {
				t.Fatalf("HTTP consumer reaches backend owner: %s", name)
			}
		}
		if count == 0 {
			t.Fatal("empty dependency closure")
		}
	}

}
