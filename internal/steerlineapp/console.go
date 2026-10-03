package steerlineapp

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/console"
)

// PrepareConsole constructs each project's sole product adapter outside the UI.
func PrepareConsole(ctx context.Context, configPath, runID string) ([]api.ProjectRef, console.Options, error) {
	refs, err := LoadConsoleProjects(configPath)
	options := console.Options{RunID: runID, Clients: map[string]api.ConsoleClient{}, ApplicationHome: HomeDir(), ApplicationBin: BinDir(), ApplicationEtc: EtcDir()}
	if err != nil {
		return nil, options, err
	}
	for i := range refs {
		if refs[i].Error != "" {
			continue
		}
		c, err := openProjectClient(ctx, refs[i])
		if err != nil {
			refs[i].Error = err.Error()
			continue
		}
		ref := c.Project()
		ref.Name, ref.Key, ref.Conflict = refs[i].Name, refs[i].Key, refs[i].Conflict
		refs[i] = ref
		options.Clients[ref.Key] = c
	}
	return refs, options, nil
}
func RunConsole(ctx context.Context, configPath, runID string) (err error) {
	refs, options, err := PrepareConsole(ctx, configPath, runID)
	if err != nil {
		return err
	}
	defer func() {
		for _, c := range options.Clients {
			err = errors.Join(err, c.Close())
		}
	}()
	return console.Run(ctx, refs, options)
}

type consoleConfigFile struct {
	Name, Path string
	Err        error
}

func consoleConfigFiles() ([]consoleConfigFile, error) {
	projects, err := ListProjects()
	if err != nil {
		return nil, err
	}
	var files []consoleConfigFile
	for _, p := range projects {
		files = append(files, consoleConfigFile{p.Name, p.ConfigPath, p.Err})
	}
	return files, nil
}
func consoleApplicationPaths() (string, string, string) { return HomeDir(), BinDir(), EtcDir() }
func consoleViewKey(configPath, runDir string) (string, error) {
	p, err := filepath.Abs(configPath)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(runDir) == "" {
		return consoleCanonical(p) + "\x00", nil
	}
	r, err := filepath.Abs(runDir)
	if err != nil {
		return "", err
	}
	return consoleCanonical(p) + "\x00" + consoleCanonical(r), nil
}
func consoleCanonical(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

// LoadProjects reads either one explicit config or all configs under etc.
// Invalid project files stay in the list so their errors remain visible.
func LoadConsoleProjects(configPath string) ([]api.ProjectRef, error) {

	var refs []api.ProjectRef
	if strings.TrimSpace(configPath) != "" {
		absolute, err := filepath.Abs(configPath)
		if err != nil {
			return nil, err
		}
		config, loadErr := LoadQueryConfig(absolute)
		name := strings.TrimSuffix(filepath.Base(absolute), filepath.Ext(absolute))
		if loadErr != nil {
			return []api.ProjectRef{invalidConsoleProjectRef(name, absolute, loadErr)}, nil
		}
		home, bin, _ := consoleApplicationPaths()
		refs = append(refs, makeConsoleProjectRef(name, absolute, config, home, bin))
	} else {
		projects, err := consoleConfigFiles()
		if err != nil {
			return nil, err
		}
		for _, project := range projects {
			path, _ := filepath.Abs(project.Path)
			if project.Err != nil {
				refs = append(refs, invalidConsoleProjectRef(project.Name, path, project.Err))
				continue
			}
			config, loadErr := LoadQueryConfig(path)
			if loadErr != nil {
				refs = append(refs, invalidConsoleProjectRef(project.Name, path, loadErr))
				continue
			}
			home, bin, _ := consoleApplicationPaths()
			refs = append(refs, makeConsoleProjectRef(project.Name, path, config, home, bin))
		}
	}
	identityNames := map[string][]string{}
	for i := range refs {
		if refs[i].Identity != "" {
			key := consoleCanonical(refs[i].Identity)
			identityNames[key] = append(identityNames[key], refs[i].Name)
		}
	}
	for i := range refs {
		if refs[i].Identity == "" {
			continue
		}
		names := identityNames[consoleCanonical(refs[i].Identity)]
		if len(names) > 1 {
			refs[i].Conflict = "同仓库配置关联/冲突：" + strings.Join(names, ", ") + "；共享单仓库服务锁，控制操作已禁用"
		}
	}
	return refs, nil
}

func invalidConsoleProjectRef(name, path string, configErr error) api.ProjectRef {
	key, _ := consoleViewKey(path, "")
	return api.ProjectRef{Key: key, Name: name, ConfigPath: path, Error: configErr.Error(), Presentation: api.ProjectPresentation{ServiceInstruction: "steerline start-service -config " + path}}
}

func makeConsoleProjectRef(name, path string, config steerline.Config, home, bin string) api.ProjectRef {
	absoluteRunDir, _ := filepath.Abs(config.RunDir)
	configPath, _ := filepath.Abs(path)
	key, _ := consoleViewKey(configPath, absoluteRunDir)
	identity := ""
	if err := config.ResolveIdentity(context.Background()); err != nil {
		return api.ProjectRef{Key: key, Name: name, ConfigPath: configPath, Presentation: projectPresentation(config), RunDir: absoluteRunDir, Error: "resolve repository identity: " + err.Error()}
	}
	identity = config.Identity()
	ref := api.ProjectRef{Key: key, Name: name, ConfigPath: configPath, Presentation: projectPresentation(config), ApplicationHome: home, ApplicationBin: bin, Identity: identity, RunDir: absoluteRunDir}
	ref.Presentation.ServiceInstruction = clientServiceInstruction(ref)
	return ref
}

func selectLogNormalizer(first []byte) steerline.Normalizer {
	normalizer := steerline.NewPiNormalizer()
	if normalizer.Detect(first) {
		return normalizer
	}
	return codexworker.NewNormalizer()
}
