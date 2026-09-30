package workerflow

import "testing"

func TestSelectModeRequiresFixedProtocolAndHost(t *testing.T) {
	for _, test := range []struct {
		protocol string
		host     string
		want     Mode
	}{
		{"local_codex_app_server_v1", "local", LocalCodex},
		{"orca_terminal_v2", "orca", NativeOrca},
		{"orca_supervised", "orca_supervised", SupervisedOrca},
	} {
		got, err := SelectMode(test.protocol, test.host)
		if err != nil || got != test.want {
			t.Fatalf("SelectMode(%q, %q) = %v, %v", test.protocol, test.host, got, err)
		}
		if _, err := SelectMode(test.protocol, "wrong-host"); err == nil {
			t.Fatalf("protocol %q accepted another host", test.protocol)
		}
	}
	turn, command := LocalCodex.PendingIdentity("fixed-request")
	if turn != "" || command != "fixed-request" {
		t.Fatal("Codex command was treated as a native turn receipt")
	}
}
