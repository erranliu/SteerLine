package query

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func DecodeActivityID(project, id string) (string, string, error) {
	if len(id) > 1024 {
		return "", "", errors.New("invalid activity identity")
	}
	raw, err := base64.RawURLEncoding.DecodeString(id)
	parts := strings.Split(string(raw), "\x00")
	if err != nil || len(parts) != 3 || parts[0] != project || parts[2] == "" {
		return "", "", errors.New("activity identity differs from project")
	}
	for _, source := range AsyncActivitySources {
		if parts[1] == source {
			return source, parts[2], nil
		}
	}
	return "", "", errors.New("unknown activity source")
}

func ReadOneActivity(ctx context.Context, reader ports.AsyncActivityReader, project, instance, id, public string) (api.AsyncActivity, error) {
	source, key, err := DecodeActivityID(project, id)
	if err != nil {
		return api.AsyncActivity{}, err
	}
	facts, _, err := reader.ReadAsyncActivityFacts(ctx, project, ports.AsyncActivityRead{Source: source, SourceID: key, Limit: 1})
	if err != nil {
		return api.AsyncActivity{}, err
	}
	if len(facts) != 1 {
		return api.AsyncActivity{}, errors.New("activity not found")
	}
	a, err := ProjectAsyncActivity(project, instance, facts[0], time.Now().UTC())
	if public != "" {
		a.ProjectID = public
	}
	return a, err
}

type ReadingService struct {
	Reader            ports.ActivityReadingReader
	Project, Instance string
	Bodies            *ReadingBodyCache
}
type readingCursor struct {
	Project, Instance, Activity, Version string
	After, End                           int64
}

func (q ReadingService) Page(ctx context.Context, id, cursor string, limit int) (api.ReadingPage, error) {
	source, key, err := DecodeActivityID(q.Project, id)
	if err != nil {
		return api.ReadingPage{}, err
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 200 {
		return api.ReadingPage{}, errors.New("invalid reading limit")
	}
	version, err := q.Reader.ReadingVersion(ctx, q.Project, source, key)
	if err != nil {
		return api.ReadingPage{}, err
	}
	c := readingCursor{Project: q.Project, Instance: q.Instance, Activity: id, Version: version}
	if cursor != "" {
		if len(cursor) > 4096 {
			return api.ReadingPage{}, errors.New("invalid reading cursor")
		}
		raw, e := base64.RawURLEncoding.DecodeString(cursor)
		if e != nil || json.Unmarshal(raw, &c) != nil || c.Project != q.Project || c.Instance != q.Instance || c.Activity != id || c.After < 0 || c.End < c.After || c.Version != version {
			return api.ReadingPage{}, errors.New("交互分页范围已变化，请刷新交互")
		}
	}
	w, err := q.Reader.ReadReadingWindow(ctx, q.Project, source, key, c.After, c.End, limit)
	if err != nil {
		return api.ReadingPage{}, err
	}
	page := api.ReadingPage{ActivityID: id, Version: version, Items: []api.ReadingItem{}, HasMore: w.More, Diagnostic: "仅显示已记录的交互，不是完整原生会话；未保存的问题和运行中消息可能缺失。"}
	bytes := 0
	for _, r := range w.Records {
		if !readableReadingRecord(r) {
			c.After = r.Seq
			continue
		}
		item := readingItem(source, r)
		raw, _ := json.Marshal(item)
		if len(raw) > 1<<20 {
			return api.ReadingPage{}, errors.New("交互索引单条记录超出预算")
		}
		if bytes+len(raw) > 1<<20 {
			page.HasMore = true
			break
		}
		bytes += len(raw)
		page.Items = append(page.Items, item)
		c.After = r.Seq
	}
	if page.HasMore {
		c.End = w.End
		raw, _ := json.Marshal(c)
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	if source == "baseline_check" {
		page.Diagnostic = "此活动只有操作记录，没有模型对话。"
	}
	return page, nil
}

// Product wording and grouping stay here; storage returns source identities,
// field kinds, original document names and round numbers only.
func readableReadingRecord(r ports.ReadingRecord) bool {
	if r.Role == "user" {
		switch r.Kind {
		case "draft", "answer", "feedback", "reply", "message":
			return true
		}
	}
	if r.Role == "assistant" {
		switch r.Kind {
		case "verified_final", "generated_result", "suggestion", "question_context":
			return true
		}
	}
	return false
}

func readingItem(source string, r ports.ReadingRecord) api.ReadingItem {
	label, group := "已记录内容", "原记录"
	switch r.Kind {
	case "draft":
		label = "用户草稿"
	case "answer", "feedback", "reply", "message":
		label = "用户输入"
	case "verified_final":
		label = "已核实的最终回复"
	case "generated_result":
		label = "生成结果"
	case "suggestion":
		label = "生成的提示词建议 · 尚未发送"
	case "question_context":
		label = "已保存问题上下文"
	}
	if r.Label != "" {
		label += " · " + r.Label
	}
	if r.AttemptID != "" {
		group = "Attempt " + r.AttemptID
		if r.TurnID != "" {
			group += " · turn " + r.TurnID
		} else {
			group += " · turn 未记录"
		}
	} else if r.Role == "user" && source == "task_candidate" {
		group = "轮次未记录"
		label += " · 轮次未记录"
	}
	if source == "environment_detection" {
		group = "检测轮次 " + r.Group
	}
	return api.ReadingItem{ID: r.ID, Role: r.Role, Kind: r.Kind, Label: label, Preview: r.Preview, Bytes: r.Bytes, SHA256: r.SHA256, At: r.At, State: r.State, RunID: r.RunID, AttemptID: r.AttemptID, SessionID: r.SessionID, TurnID: r.TurnID, Group: group}
}

func (q ReadingService) Version(ctx context.Context, id string) (string, error) {
	s, k, e := DecodeActivityID(q.Project, id)
	if e != nil {
		return "", e
	}
	return q.Reader.ReadingVersion(ctx, q.Project, s, k)
}

func (q ReadingService) Body(ctx context.Context, id, item, digest string, offset int) (api.ReadingBody, error) {
	source, key, err := DecodeActivityID(q.Project, id)
	if err != nil {
		return api.ReadingBody{}, err
	}
	r, err := q.Reader.ReadingBodyIdentity(ctx, q.Project, source, key, item)
	if err != nil {
		return api.ReadingBody{}, err
	}
	if len(digest) != 64 || r.SHA256 != digest || r.Bytes > 16<<20 || r.Bytes < 1 || offset < 0 || offset > r.Bytes {
		return api.ReadingBody{}, errors.New("正文引用已失效或超出预算")
	}
	if q.Bodies == nil {
		return api.ReadingBody{}, errors.New("reading body cache unavailable")
	}
	body, err := q.Bodies.read(ctx, q.Project+"\x00"+q.Instance+"\x00"+id+"\x00"+item+"\x00"+digest, func() (string, error) {
		text, e := q.Reader.ReadReadingBody(ctx, q.Project, source, key, item, 16<<20)
		if e != nil {
			return "", e
		}
		sum := sha256.Sum256([]byte(text))
		if !utf8.ValidString(text) || len(text) != r.Bytes || hex.EncodeToString(sum[:]) != digest {
			return "", errors.New("原正文与引用不符")
		}
		return text, nil
	})
	if err != nil {
		return api.ReadingBody{}, err
	}
	if offset < len(body) && body[offset]&0xC0 == 0x80 {
		return api.ReadingBody{}, errors.New("invalid UTF-8 body offset")
	}
	end := min(len(body), offset+(256<<10))
	for end < len(body) && body[end]&0xC0 == 0x80 {
		end--
	}
	return api.ReadingBody{ID: item, SHA256: digest, Offset: offset, NextOffset: end, Bytes: len(body), Text: body[offset:end], Complete: end == len(body)}, nil
}

type readingCacheEntry struct {
	body string
	used time.Time
}
type readingLoad struct {
	done chan struct{}
	body string
	err  error
}

// ReadingBodyCache is injected once per application; it owns no SQL or writer.
type ReadingBodyCache struct {
	mu      sync.Mutex
	entries map[string]readingCacheEntry
	loads   map[string]*readingLoad
	bytes   int
	slots   chan struct{}
}

func NewReadingBodyCache() *ReadingBodyCache {
	return &ReadingBodyCache{entries: map[string]readingCacheEntry{}, loads: map[string]*readingLoad{}, slots: make(chan struct{}, 2)}
}
func (c *ReadingBodyCache) read(ctx context.Context, key string, load func() (string, error)) (string, error) {
	c.mu.Lock()
	now := time.Now()
	for k, v := range c.entries {
		if now.Sub(v.used) > 5*time.Minute {
			delete(c.entries, k)
			c.bytes -= len(v.body)
		}
	}
	if v, ok := c.entries[key]; ok {
		v.used = now
		c.entries[key] = v
		c.mu.Unlock()
		return v.body, nil
	}
	if running, ok := c.loads[key]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-running.done:
			return running.body, running.err
		}
	}
	running := &readingLoad{done: make(chan struct{})}
	c.loads[key] = running
	c.mu.Unlock()
	select {
	case c.slots <- struct{}{}:
		running.body, running.err = load()
		<-c.slots
	case <-ctx.Done():
		running.err = ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.loads, key)
	if running.err == nil {
		for len(c.entries) >= 128 || c.bytes+len(running.body) > 32<<20 {
			old := ""
			var at time.Time
			for k, v := range c.entries {
				if old == "" || v.used.Before(at) {
					old = k
					at = v.used
				}
			}
			if old == "" {
				running.err = fmt.Errorf("body cache capacity exceeded")
				break
			}
			c.bytes -= len(c.entries[old].body)
			delete(c.entries, old)
		}
		if running.err == nil {
			c.entries[key] = readingCacheEntry{running.body, time.Now()}
			c.bytes += len(running.body)
		}
	}
	close(running.done)
	return running.body, running.err
}
