package query

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/storage/ports"
)

type readingFixture struct {
	body                        string
	reads, versions, identities int
	version                     string
	records                     []ports.ReadingRecord
}

func (f *readingFixture) ReadingVersion(context.Context, string, string, string) (string, error) {
	f.versions++
	return f.version, nil
}
func (f *readingFixture) ReadReadingWindow(_ context.Context, _, _, _ string, after, end int64, limit int) (ports.ReadingWindow, error) {
	if end == 0 {
		end = int64(len(f.records))
	}
	w := ports.ReadingWindow{End: end}
	for _, r := range f.records {
		if r.Seq > after && r.Seq <= end {
			if len(w.Records) == limit {
				w.More = true
				break
			}
			w.Records = append(w.Records, r)
		}
	}
	return w, nil
}
func (f *readingFixture) ReadingBodyIdentity(context.Context, string, string, string, string) (ports.ReadingRecord, error) {
	f.identities++
	sum := sha256.Sum256([]byte(f.body))
	return ports.ReadingRecord{ID: "item", Bytes: len(f.body), SHA256: hex.EncodeToString(sum[:])}, nil
}
func (f *readingFixture) ReadReadingBody(context.Context, string, string, string, string, int) (string, error) {
	f.reads++
	return f.body, nil
}
func readingTestID(project string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(project + "\x00task_candidate\x00candidate"))
}

func TestActivityReadingBodyVerifiesOnceAndReadsOnlyRequestedSegments(t *testing.T) {
	f := &readingFixture{body: strings.Repeat("可读消息", 70000), version: "1"}
	q := ReadingService{Reader: f, Project: "p", Instance: "i", Bodies: NewReadingBodyCache()}
	sum := sha256.Sum256([]byte(f.body))
	digest := hex.EncodeToString(sum[:])
	offset := 0
	var text strings.Builder
	for {
		page, err := q.Body(context.Background(), readingTestID("p"), "item", digest, offset)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Text) > 256<<10 || page.Offset != offset || page.NextOffset != offset+len(page.Text) {
			t.Fatalf("invalid chunk: %+v", page)
		}
		text.WriteString(page.Text)
		offset = page.NextOffset
		if page.Complete {
			break
		}
	}
	if text.String() != f.body || f.reads != 1 || f.identities < 2 {
		t.Fatalf("repeated body materialization or missing per-request identity: reads=%d identities=%d", f.reads, f.identities)
	}
	if _, err := q.Body(context.Background(), readingTestID("other"), "item", digest, 0); err == nil {
		t.Fatal("cross-project body accepted")
	}
	if _, err := q.Body(context.Background(), readingTestID("p"), "item", strings.Repeat("0", 64), 0); err == nil {
		t.Fatal("old digest accepted")
	}
	if _, err := q.Body(context.Background(), readingTestID("p"), "item", digest, 1); err == nil {
		t.Fatal("split UTF-8 offset accepted")
	}
}

func TestActivityReadingIndexIsOnePageAndCursorBindsVersionAndInstance(t *testing.T) {
	f := &readingFixture{version: "1", records: []ports.ReadingRecord{{Seq: 1, ID: "first", Role: "user", Kind: "answer", Preview: "answer", Group: "轮次未记录"}, {Seq: 2, ID: "second", Role: "assistant", Kind: "verified_final", Preview: "final"}}}
	q := ReadingService{Reader: f, Project: "p", Instance: "i", Bodies: NewReadingBodyCache()}
	first, err := q.Page(context.Background(), readingTestID("p"), "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || !first.HasMore || first.NextCursor == "" || f.reads != 0 {
		t.Fatalf("index read body or auto paged: %+v", first)
	}
	second, err := q.Page(context.Background(), readingTestID("p"), first.NextCursor, 1)
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != "second" || second.HasMore {
		t.Fatalf("second page: %+v %v", second, err)
	}
	q.Instance = "new"
	if _, err = q.Page(context.Background(), readingTestID("p"), first.NextCursor, 1); err == nil {
		t.Fatal("old instance cursor accepted")
	}
	q.Instance = "i"
	f.version = "2"
	if _, err = q.Page(context.Background(), readingTestID("p"), first.NextCursor, 1); err == nil {
		t.Fatal("changed source cursor accepted")
	}
	before := f.reads
	if _, err = q.Version(context.Background(), readingTestID("p")); err != nil || f.reads != before {
		t.Fatal("changes read body")
	}
}

func TestActivityReadingCacheEvictsAndRevalidates(t *testing.T) {
	c := NewReadingBodyCache()
	loads := 0
	load := func() (string, error) { loads++; return strings.Repeat("x", 12<<20), nil }
	for _, key := range []string{"a", "b", "c", "a"} {
		if _, err := c.read(context.Background(), key, load); err != nil {
			t.Fatal(err)
		}
	}
	if loads != 4 || c.bytes > 32<<20 {
		t.Fatalf("cache unbounded or failed to revalidate: %d %d", loads, c.bytes)
	}
}

func TestActivityReadingKeepsPresentationOutOfStorageAndExcludesTools(t *testing.T) {
	f := &readingFixture{version: "1", records: []ports.ReadingRecord{{Seq: 1, ID: "tool", Role: "tool", Kind: "tool_call", Preview: "private output"}, {Seq: 2, ID: "input", Role: "user", Kind: "answer", Preview: "saved answer", State: "unknown"}, {Seq: 3, ID: "unknown", Role: "assistant", Kind: "internal", Preview: "technical event"}}}
	q := ReadingService{Reader: f, Project: "p", Instance: "i", Bodies: NewReadingBodyCache()}
	page, err := q.Page(context.Background(), readingTestID("p"), "", 50)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "input" || page.Items[0].Group != "轮次未记录" || !strings.Contains(page.Items[0].Label, "用户输入") || f.reads != 0 {
		t.Fatalf("reading projection: %+v %v", page, err)
	}
}
