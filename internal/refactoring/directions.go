package refactoring

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/refactoring/model"
)

const customPrefix = "custom:"
const maxDirectionBytes = 32 << 10
const customDirectionsDir = "etc/refactoring-directions"

var builtInDirections = []string{"simplify", "cleanup", "boundaries", "readability", "docs-tests"}

var ErrDirectionChanged = errors.New("重构提示词已更新，请刷新并查看最新正文后重新启动")

type DirectionCatalogue struct {
	Directions  []model.Direction `json:"directions"`
	Diagnostics []string          `json:"diagnostics,omitempty"`
}

func openCustomDirections(home string) (*os.Root, error) {
	root, err := os.OpenRoot(home)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.OpenRoot(filepath.FromSlash(customDirectionsDir))
}

func ListDirections(home string) DirectionCatalogue {
	catalogue := DirectionCatalogue{Directions: []model.Direction{}}
	add := func(direction model.Direction, err error) {
		if err != nil {
			catalogue.Diagnostics = append(catalogue.Diagnostics, err.Error())
			return
		}
		catalogue.Directions = append(catalogue.Directions, direction)
	}
	for _, id := range builtInDirections {
		add(readBuiltInDirection(id))
	}
	root, err := openCustomDirections(home)
	if errors.Is(err, fs.ErrNotExist) {
		return catalogue
	}
	if err != nil {
		catalogue.Diagnostics = append(catalogue.Diagnostics, "无法打开自定义重构目录："+err.Error())
		return catalogue
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		catalogue.Diagnostics = append(catalogue.Diagnostics, "无法列出自定义重构目录："+err.Error())
		return catalogue
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		id := customPrefix + strings.TrimSuffix(name, ".md")
		if !model.ValidDirectionID(id) {
			catalogue.Diagnostics = append(catalogue.Diagnostics, fmt.Sprintf("自定义方向 %q 的文件名无效", name))
			continue
		}
		add(readCustomDirection(root, id))
	}
	return catalogue
}

func ResolveDirection(home, id, revision string) (model.Direction, error) {
	var direction model.Direction
	var err error
	if slices.Contains(builtInDirections, id) {
		direction, err = readBuiltInDirection(id)
	} else if model.ValidDirectionID(id) && strings.HasPrefix(id, customPrefix) {
		root, openErr := openCustomDirections(home)
		if openErr != nil {
			return direction, fmt.Errorf("无法打开自定义重构目录，请刷新后重试：%w", openErr)
		}
		defer root.Close()
		entries, readErr := fs.ReadDir(root.FS(), ".")
		if readErr != nil {
			return direction, fmt.Errorf("无法列出自定义重构目录，请刷新后重试：%w", readErr)
		}
		name := customDirectionFilename(id)
		if !slices.ContainsFunc(entries, func(entry fs.DirEntry) bool { return entry.Name() == name }) {
			return direction, fmt.Errorf("重构方向 %q 不存在，请刷新后重试", id)
		}
		direction, err = readCustomDirection(root, id)
	} else {
		return direction, fmt.Errorf("重构方向 %q 不存在，请刷新后重试", id)
	}
	if err != nil {
		return direction, fmt.Errorf("无法使用所选重构方向，请刷新后重试：%w", err)
	}
	if direction.Revision != revision {
		return model.Direction{}, ErrDirectionChanged
	}
	return direction, nil
}

func readBuiltInDirection(id string) (model.Direction, error) {
	raw, err := prompts.MaintenanceDirection(id)
	if err != nil {
		return model.Direction{}, fmt.Errorf("读取内置方向 %s：%w", id, err)
	}
	return parseDirection(id, raw)
}

func customDirectionFilename(id string) string {
	return strings.TrimPrefix(id, customPrefix) + ".md"
}

func readCustomDirection(root *os.Root, id string) (model.Direction, error) {
	name := customDirectionFilename(id)
	info, err := root.Lstat(name)
	if err != nil {
		return model.Direction{}, fmt.Errorf("读取自定义方向 %s：%w", name, err)
	}
	if !info.Mode().IsRegular() {
		return model.Direction{}, fmt.Errorf("自定义方向 %s 必须是普通文件", name)
	}
	file, err := root.Open(name)
	if err != nil {
		return model.Direction{}, fmt.Errorf("读取自定义方向 %s：%w", name, err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxDirectionBytes+1))
	if err != nil {
		return model.Direction{}, fmt.Errorf("读取自定义方向 %s：%w", name, err)
	}
	return parseDirection(id, raw)
}

func parseDirection(id string, raw []byte) (model.Direction, error) {
	if len(raw) > maxDirectionBytes || !utf8.Valid(raw) {
		return model.Direction{}, fmt.Errorf("方向 %s 必须使用 UTF-8，且不超过 32 KiB", id)
	}
	prompt := string(raw)
	title, body, _ := strings.Cut(strings.TrimPrefix(prompt, "\ufeff"), "\n")
	title, valid := strings.CutPrefix(title, "# ")
	title = strings.TrimSpace(title)
	if !valid || title == "" || strings.TrimSpace(body) == "" {
		return model.Direction{}, fmt.Errorf("方向 %s 需要在首行填写一级标题，并提供提示词正文", id)
	}
	return model.Direction{ID: id, Name: title, Prompt: prompt, Revision: model.Digest(raw)}, nil
}
