package workerreturn

import (
	"strings"
	"testing"
)

func TestTerminalFrameBindsReplyAndIgnoresInputEcho(t *testing.T) {
	command := strings.Repeat("a", 32)
	prompt, err := Prompt("Format the preceding result as JSON.", command)
	if err != nil {
		t.Fatal(err)
	}
	var scan Scan
	scan.Append(prompt, command)
	if scan.Result != nil {
		t.Fatal("input echo became a worker result")
	}
	reply := Reply{Outcome: "succeeded", Text: `{"outcome":"done","summary":"complete"}`}
	frame, err := Encode(command, reply)
	if err != nil {
		t.Fatal(err)
	}
	scan.Append(frame[:len(frame)/2], command)
	if scan.Result != nil {
		t.Fatal("incomplete frame accepted")
	}
	scan.Append(frame[len(frame)/2:], command)
	scan.Append(frame, command)
	if scan.Result == nil || *scan.Result != reply || scan.Validate(command) != nil {
		t.Fatal("original reply missing", scan)
	}
	other, err := Encode(command, Reply{Outcome: "failed", Text: "different"})
	if err != nil {
		t.Fatal(err)
	}
	scan.Append(other, command)
	if scan.Problem == "" {
		t.Fatal("conflicting reply overwrote original")
	}
}

func TestTerminalFrameRejectsForeignAndAmbiguousResults(t *testing.T) {
	command := strings.Repeat("a", 32)
	foreign, _ := Encode(strings.Repeat("b", 32), Reply{Outcome: "succeeded", Text: "foreign"})
	var scan Scan
	scan.Append(foreign, command)
	if scan.Result != nil {
		t.Fatal("another input's reply accepted")
	}
	for _, body := range []string{
		`{"nonce":"` + command + `","outcome":"succeeded","outcome":"failed","reply":"text"}`,
		`{"nonce":"` + command + `","outcome":"unknown","reply":"text"}`,
		`{"nonce":"` + command + `","outcome":"succeeded","reply":"text","extra":1}`,
	} {
		var scan Scan
		scan.Append("SL_RESULT "+body+" SL_END", command)
		if scan.Result != nil || scan.Problem == "" {
			t.Fatal("invalid result accepted", body)
		}
	}
	if _, err := Prompt(strings.Repeat("x", MaxPromptRunes), command); err == nil {
		t.Fatal("oversized input accepted")
	}
}

func TestTerminalFrameScreenWrapPreservesExactReply(t *testing.T) {
	command := strings.Repeat("c", 32)
	reply := Reply{Outcome: "succeeded", Text: `{"outcome":"done","outputs":{"pr_url":"https://github.com/erranliu/SteerlineTest/pull/199"},"summary":"完整 reply"}`}
	frame, err := Encode(command, reply)
	if err != nil {
		t.Fatal(err)
	}
	for _, position := range []int{strings.Index(frame, command) + 8, strings.Index(frame, "reply_b64") + 25} {
		var scan Scan
		scan.Append(frame[:position]+"  "+frame[position:], command)
		if scan.Result == nil || *scan.Result != reply {
			t.Fatal("display indentation changed reply", scan)
		}
	}
}
