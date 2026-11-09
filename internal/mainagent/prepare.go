package mainagent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/internal/mainagent/model"
	"github.com/erranliu/SteerLine/internal/pathguard"
	"github.com/erranliu/SteerLine/internal/prompts"
)

// PrepareRules is explicit and non-overwriting. The file must subsequently be
// reviewed, committed and published before any analysis may consume it.
func PrepareRules(repository string, check bool) error {
	if !filepath.IsAbs(repository) {
		return errors.New("absolute repository required")
	}
	target := filepath.Join(repository, filepath.FromSlash(model.RulesPath))
	resolved, err := pathguard.ResolveProspective(target)
	if err != nil || !strings.EqualFold(filepath.Clean(target), filepath.Clean(resolved)) {
		return errors.New("main Agent rule path redirected")
	}
	expected, err := prompts.MainAgentRules()
	if err != nil {
		return err
	}
	if info, e := os.Lstat(target); e == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("main Agent rules are not a regular file")
		}
		body, e := os.ReadFile(target)
		if e != nil {
			return e
		}
		if !prompts.SameRules(body, expected) {
			return errors.New("主 Agent 规则不同源，请审阅更新原文件")
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	if check {
		return errors.New("缺少主 Agent 规则；请显式准备后提交并合入目标分支")
	}
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	resolved, err = pathguard.ResolveProspective(target)
	if err != nil || !strings.EqualFold(target, resolved) {
		return errors.New("main Agent rule path changed")
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(expected)
	return errors.Join(err, f.Sync(), f.Close())
}
