package file_reader

import (
	"context"
	"fmt"
	"strings"

	"github.com/werf/werf/pkg/giterminism"
)

var DefaultWerfConfigNames = []string{"werf.yaml", "werf.yml"}

func (r FileReader) ReadConfig(ctx context.Context, customRelPath string) ([]byte, error) {
	var configRelPathList []string
	if customRelPath != "" {
		configRelPathList = append(configRelPathList, customRelPath)
	} else {
		configRelPathList = DefaultWerfConfigNames
	}

	for _, configPath := range configRelPathList {
		if exist, err := r.isConfigExist(ctx, configPath); err != nil {
			return nil, err
		} else if !exist {
			continue
		}

		return r.readConfig(ctx, configPath)
	}

	return nil, r.prepareConfigNotFoundError(configRelPathList)
}

func (r FileReader) isConfigExist(ctx context.Context, relPath string) (bool, error) {
	if r.manager.LooseGiterminism() || r.manager.Config().IsUncommittedConfigAccepted() {
		return r.isFileExist(relPath)
	}

	return r.isCommitFileExist(ctx, relPath)
}

func (r FileReader) readConfig(ctx context.Context, relPath string) ([]byte, error) {
	if r.manager.LooseGiterminism() || r.manager.Config().IsUncommittedConfigAccepted() {
		return r.readFile(relPath)
	}

	return r.readCommitConfig(ctx, relPath)
}

func (r FileReader) readCommitConfig(ctx context.Context, relPath string) ([]byte, error) {
	return r.readCommitFile(ctx, relPath, func(ctx context.Context, relPath string) error {
		return giterminism.NewUncommittedConfigurationError(fmt.Sprintf("the werf config '%s' must be committed", relPath))
	})
}

func (r FileReader) prepareConfigNotFoundError(configPathsToCheck []string) error {
	if r.manager.LooseGiterminism() {
		return giterminism.NewConfigNotFoundError(fmt.Sprintf("the werf config '%s' not found in the project directory", strings.Join(configPathsToCheck, "', '")))
	}

	return giterminism.NewConfigNotFoundError(fmt.Sprintf("the werf config '%s' not found in the project git repository", strings.Join(configPathsToCheck, "', '")))
}
