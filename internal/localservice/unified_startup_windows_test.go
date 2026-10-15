//go:build windows

package localservice

import (
	"fmt"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnifiedStartupPortConflictReportsOriginalOwner(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	home := t.TempDir()
	_, err = Prepare(home, filepath.Join(home, "web"), contract.WebProtocol, listener.Addr().String())
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("pid=%d", os.Getpid())) || !strings.Contains(err.Error(), listener.Addr().String()) {
		t.Fatal("port conflict did not identify the original listener", err)
	}
}
