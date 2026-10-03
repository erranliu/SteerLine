package steerlineapp

import (
	"context"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/console"
)

func ConsoleOptions(runID string) console.Options {
	return console.Options{RunID: runID, Query: console.FileQuerySource{Factory: ClientResources}, Control: console.FileControlClient{Factory: ClientResources}, ApplicationHome: HomeDir(), ApplicationBin: BinDir(), ApplicationEtc: EtcDir()}
}

func RunConsole(ctx context.Context, configPath, runID string) error {
	projects, err := console.LoadProjects(configPath, ConsoleConfigSource{})
	if err != nil {
		return err
	}
	return console.Run(ctx, projects, ConsoleOptions(runID))
}

type ConsoleConfigSource struct{}

func (ConsoleConfigSource) ConfigFiles() ([]console.ProjectConfigFile, error) {
	projects, err := ListProjects()
	if err != nil {
		return nil, err
	}
	files := make([]console.ProjectConfigFile, 0, len(projects))
	for _, project := range projects {
		files = append(files, console.ProjectConfigFile{Name: project.Name, Path: project.ConfigPath, Err: project.Err})
	}
	return files, nil
}

func (ConsoleConfigSource) LoadConfig(path string) (steerline.Config, error) {
	return LoadQueryConfig(path)
}

func (ConsoleConfigSource) ApplicationPaths() (string, string, string) {
	return HomeDir(), BinDir(), EtcDir()
}

func selectLogNormalizer(first []byte) steerline.Normalizer {
	normalizer := steerline.NewPiNormalizer()
	if normalizer.Detect(first) {
		return normalizer
	}
	return codexworker.NewNormalizer()
}
