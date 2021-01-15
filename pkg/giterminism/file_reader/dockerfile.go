package file_reader

import (
	"context"
)

func (r FileReader) ReadDockerfile(ctx context.Context, relPath string) ([]byte, error) {
	if err := r.checkDockerfileExistence(ctx, relPath); err != nil {
		return nil, err
	}

	return r.readDockerfile(ctx, relPath)
}

func (r FileReader) readDockerfile(ctx context.Context, relPath string) ([]byte, error) {
	return r.readConfigurationFile(ctx, dockerfileErrorConfigType, relPath, r.manager.Config().IsUncommittedDockerfileAccepted)
}

func (r FileReader) checkDockerfileExistence(ctx context.Context, relPath string) error {
	return r.checkConfigurationFileExistence(ctx, dockerfileErrorConfigType, relPath, r.manager.Config().IsUncommittedDockerfileAccepted)
}

func (r FileReader) IsDockerignoreExistAnywhere(ctx context.Context, relPath string) (bool, error) {
	if exist, err := r.isCommitFileExist(ctx, relPath); err != nil {
		return false, err
	} else if !exist {
		return r.isFileExist(relPath)
	} else {
		return true, nil
	}
}

func (r FileReader) ReadDockerignore(ctx context.Context, relPath string) ([]byte, error) {
	if err := r.checkDockerignoreExistence(ctx, relPath); err != nil {
		return nil, err
	}

	return r.readDockerignore(ctx, relPath)
}

func (r FileReader) checkDockerignoreExistence(ctx context.Context, relPath string) error {
	return r.checkConfigurationFileExistence(ctx, dockerignoreErrorConfigType, relPath, r.manager.Config().IsUncommittedDockerignoreAccepted)
}

func (r FileReader) isDockerignoreExist(ctx context.Context, relPath string) (bool, error) {
	return r.isConfigurationFileExist(ctx, relPath, r.manager.Config().IsUncommittedDockerignoreAccepted)
}

func (r FileReader) readDockerignore(ctx context.Context, relPath string) ([]byte, error) {
	return r.readConfigurationFile(ctx, dockerignoreErrorConfigType, relPath, r.manager.Config().IsUncommittedDockerignoreAccepted)
}
