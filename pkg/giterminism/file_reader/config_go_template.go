package file_reader

import (
	"context"
	"fmt"
	"path/filepath"
)

func (r FileReader) ConfigGoTemplateFilesGlob(ctx context.Context, pattern string) (map[string]interface{}, error) {
	result := map[string]interface{}{}

	if err := r.configurationFilesGlob(
		ctx,
		configGoTemplateErrorConfigType,
		pattern,
		r.manager.Config().IsUncommittedConfigGoTemplateRenderingFileAccepted,
		r.readCommitConfigGoTemplateFile,
		func(relPath string, data []byte, err error) error {
			if err != nil {
				return err
			}

			result[filepath.ToSlash(relPath)] = string(data)

			return nil
		},
	); err != nil {
		return nil, fmt.Errorf("{{ .Files.Glob '%s' }}: %s", pattern, err)
	}

	return result, nil
}

func (r FileReader) ConfigGoTemplateFilesGet(ctx context.Context, relPath string) ([]byte, error) {
	if err := r.checkConfigGoTemplateFileExistence(ctx, relPath); err != nil {
		return nil, fmt.Errorf("{{ .Files.Get '%s' }}: %s", relPath, err)
	}

	data, err := r.readConfigGoTemplateFile(ctx, relPath)
	if err != nil {
		return nil, fmt.Errorf("{{ .Files.Get '%s' }}: %s", relPath, err)
	}

	return data, nil
}

func (r FileReader) checkConfigGoTemplateFileExistence(ctx context.Context, relPath string) error {
	return r.checkConfigurationFileExistence(ctx, configGoTemplateErrorConfigType, relPath, r.manager.Config().IsUncommittedConfigGoTemplateRenderingFileAccepted)
}

func (r FileReader) readConfigGoTemplateFile(ctx context.Context, relPath string) ([]byte, error) {
	return r.readConfigurationFile(ctx, configGoTemplateErrorConfigType, relPath, r.manager.Config().IsUncommittedConfigGoTemplateRenderingFileAccepted)
}

func (r FileReader) readCommitConfigGoTemplateFile(ctx context.Context, relPath string) ([]byte, error) {
	return r.readCommitFile(ctx, relPath, func(ctx context.Context, relPath string) error {
		return NewUncommittedFilesChangesError(configGoTemplateErrorConfigType, relPath)
	})
}
