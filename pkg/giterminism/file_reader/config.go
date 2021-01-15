package file_reader

import (
	"context"
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

	return nil, r.prepareConfigNotFoundError(ctx, configRelPathList)
}

func (r FileReader) isConfigExist(ctx context.Context, relPath string) (bool, error) {
	return r.isConfigurationFileExist(ctx, relPath, func(_ string) (bool, error) {
		return r.manager.Config().IsUncommittedConfigAccepted(), nil
	})
}

func (r FileReader) readConfig(ctx context.Context, relPath string) ([]byte, error) {
	return r.readConfigurationFile(ctx, configErrorConfigType, relPath, func(relPath string) (bool, error) {
		return r.manager.Config().IsUncommittedConfigAccepted(), nil
	})
}

func (r FileReader) readCommitConfig(ctx context.Context, relPath string) ([]byte, error) {
	return r.readCommitFile(ctx, relPath, func(ctx context.Context, relPath string) error {
		return NewUncommittedFilesChangesError(configErrorConfigType, relPath)
	})
}

func (r FileReader) prepareConfigNotFoundError(ctx context.Context, configPathsToCheck []string) error {
	for _, configPath := range configPathsToCheck {
		err := r.checkConfigurationFileExistence(ctx, configErrorConfigType, configPath, func(_ string) (bool, error) {
			return r.manager.Config().IsUncommittedConfigAccepted(), nil
		})

		switch err.(type) {
		case UncommittedFilesError, UncommittedFilesChangesError:
			return err
		}
	}

	var configPath string
	if len(configPathsToCheck) == 1 {
		configPath = configPathsToCheck[0]
	} else { // default werf config (werf.yaml, werf.yml)
		configPath = "werf.yaml"
	}

	var err error
	if r.manager.LooseGiterminism() {
		err = NewFilesNotFoundInTheProjectDirectoryError(configErrorConfigType, configPath)
	} else {
		err = NewFilesNotFoundInTheProjectGitRepositoryError(configErrorConfigType, configPath)
	}

	return ConfigNotFoundError{err}
}

type ConfigNotFoundError struct {
	error
}

func IsConfigNotFoundError(err error) bool {
	switch err.(type) {
	case ConfigNotFoundError:
		return true
	default:
		return false
	}
}
