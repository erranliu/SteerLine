// Package consoleapp composes the terminal HTTP client without backend owners.
package consoleapp

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/erranliu/SteerLine/internal/console"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/projectconnection"
)

type Paths struct {
	Home, Bin, Executable, SourceCommit string
	SourceDirty                         string
}

func canonical(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}
func Load(ctx context.Context, configPath string, paths Paths) ([]api.ProjectRef, error) {
	var files []string
	if configPath != "" {
		absolute, err := filepath.Abs(configPath)
		if err != nil {
			return nil, err
		}
		files = []string{absolute}
	} else {
		var err error
		files, err = filepath.Glob(filepath.Join(paths.Home, "etc", "*.json"))
		if err != nil {
			return nil, err
		}
	}
	refs := make([]api.ProjectRef, 0, len(files))
	names := map[string][]string{}
	for _, file := range files {
		ref := api.ProjectRef{Name: strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)), ConfigPath: file, Key: canonical(file) + "\x00", ApplicationHome: paths.Home, ApplicationBin: paths.Bin, ExecutablePath: paths.Executable, SourceCommit: paths.SourceCommit, SourceDirty: paths.SourceDirty}
		ref.Presentation.ServiceInstruction = "steerline start-service -config " + file
		if runtime.GOOS == "windows" {
			ref.Presentation.ServiceInstruction = filepath.Join(paths.Home, "service-window.bat")
		}
		if err := projectconnection.Resolve(ctx, &ref); err != nil {
			ref.Error = err.Error()
		}
		refs = append(refs, ref)
		if ref.Identity != "" {
			names[ref.Identity] = append(names[ref.Identity], ref.Name)
		}
	}
	for i := range refs {
		if len(names[refs[i].Identity]) > 1 {
			refs[i].Conflict = "同仓库配置关联/冲突：" + strings.Join(names[refs[i].Identity], ", ") + "；控制操作已禁用"
		}
	}
	return refs, nil
}
func Prepare(ctx context.Context, configPath, runID string, paths Paths) ([]api.ProjectRef, console.Options, error) {
	refs, err := Load(ctx, configPath, paths)
	options := console.Options{RunID: runID, Clients: map[string]console.ConsoleClient{}, ApplicationHome: paths.Home, ApplicationBin: paths.Bin, ApplicationEtc: filepath.Join(paths.Home, "etc")}
	if err != nil {
		return nil, options, err
	}
	for i := range refs {
		if refs[i].Error != "" {
			continue
		}
		client := httpclient.NewConsole(ctx, refs[i], httpclient.NewPlatformStopObserver())
		refs[i] = client.Project()
		options.Clients[refs[i].Key] = client
	}
	return refs, options, nil
}
