package roadmap

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/roadmap/model"
)

// ReadRegular refuses redirection in every component, not just the leaf file.
func ReadRegular(repository, relative string) ([]byte, error) {
	if relative == "" || filepath.ToSlash(filepath.Clean(relative)) != relative || filepath.IsAbs(relative) || strings.ContainsAny(relative, "\\:\x00") || relative == ".." || strings.HasPrefix(relative, "../") {
		return nil, errors.New("必须使用仓库内规范相对路径")
	}
	root, e := filepath.Abs(repository)
	if e != nil {
		return nil, e
	}
	resolved, e := filepath.EvalSymlinks(root)
	if e != nil {
		return nil, e
	}
	target := filepath.Join(resolved, filepath.FromSlash(relative))
	cursor := resolved
	for _, part := range strings.Split(relative, "/") {
		cursor = filepath.Join(cursor, part)
		entry, e := os.Lstat(cursor)
		if e != nil {
			return nil, e
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("拒绝重定向项目文件")
		}
	}
	actual, e := filepath.EvalSymlinks(target)
	if e != nil {
		return nil, e
	}
	if !strings.EqualFold(actual, target) {
		return nil, errors.New("拒绝重定向项目文件")
	}
	info, e := os.Lstat(target)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() || info.Size() > model.MaxBytes {
		return nil, errors.New("必须是大小受限的普通文件")
	}
	b, e := os.ReadFile(target)
	if e != nil {
		return nil, e
	}
	if len(b) == 0 || len(b) > model.MaxBytes || !utf8.Valid(b) {
		return nil, errors.New("文件为空、过大或不是 UTF-8")
	}
	return b, nil
}
func ReadSource(repository, project, relative, head string) (model.Source, error) {
	s := model.Source{ID: model.SourceID(project, relative), Path: relative, Head: head}
	if !model.SourcePath(relative) {
		return s, errors.New("来源必须位于 Roadmap drafts 目录内")
	}
	b, e := ReadRegular(repository, relative)
	if e != nil {
		return s, e
	}
	s.Body = string(b)
	s.Digest = model.Digest(b)
	return s, nil
}
func ListSources(repository, project string) ([]model.Source, error) {
	dir := filepath.Join(repository, filepath.FromSlash(strings.TrimSuffix(model.Root, "/")))
	out := []model.Source{}
	if _, e := os.Stat(dir); os.IsNotExist(e) {
		return out, nil
	}
	// Refuse an aliased root before WalkDir can enumerate outside the repository.
	actual, e := filepath.EvalSymlinks(dir)
	if e != nil {
		return nil, e
	}
	absolute, e := filepath.Abs(dir)
	if e != nil {
		return nil, e
	}
	repo, e := filepath.EvalSymlinks(repository)
	if e != nil {
		return nil, e
	}
	expected := filepath.Join(repo, filepath.FromSlash(strings.TrimSuffix(model.Root, "/")))
	if !strings.EqualFold(actual, absolute) || !strings.EqualFold(actual, expected) {
		return nil, errors.New("Roadmap 目录存在重定向")
	}
	e = filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".md") {
			return nil
		}
		rel, e := filepath.Rel(repository, p)
		if e != nil {
			return e
		}
		relative := filepath.ToSlash(rel)
		s, e := ReadSource(repository, project, relative, "")
		if e != nil {
			s.Diagnostic = e.Error()
		}
		out = append(out, s)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, e
}
