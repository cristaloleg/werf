package file_reader

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
)

func (r FileReader) isCommitFileExist(ctx context.Context, relPath string) (bool, error) {
	exist, err := r.manager.LocalGitRepo().IsCommitFileExists(ctx, r.manager.HeadCommit(), relPath)
	if err != nil {
		err := fmt.Errorf(
			"unable to check existence of file %s in the project git repo commit %s: %s",
			relPath, r.manager.HeadCommit(), err,
		)
		return false, err
	}

	return exist, nil
}

func (r FileReader) readCommitFile(ctx context.Context, relPath string, handleUncommittedChangesFunc func(context.Context, string) error) ([]byte, error) {
	fileRepoPath := filepath.ToSlash(relPath)

	repoData, err := r.manager.LocalGitRepo().ReadCommitFile(ctx, r.manager.HeadCommit(), fileRepoPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read file %q from the local git repo commit %s: %s", fileRepoPath, r.manager.HeadCommit(), err)
	}

	if handleUncommittedChangesFunc == nil {
		return repoData, nil
	}

	isDataIdentical, err := r.compareFileData(relPath, repoData)
	if err != nil {
		return nil, fmt.Errorf("unable to compare commit file %q with the local project file: %s", fileRepoPath, err)
	}

	if !isDataIdentical {
		if err := handleUncommittedChangesFunc(ctx, relPath); err != nil {
			return nil, err
		}
	}

	return repoData, err
}

func (r FileReader) compareFileData(relPath string, data []byte) (bool, error) {
	var fileData []byte
	if exist, err := r.isFileExist(relPath); err != nil {
		return false, err
	} else if exist {
		fileData, err = r.readFile(relPath)
		if err != nil {
			return false, err
		}
	}

	isDataIdentical := bytes.Equal(fileData, data)
	fileDataWithForcedUnixLineBreak := bytes.ReplaceAll(fileData, []byte("\r\n"), []byte("\n"))
	if !isDataIdentical {
		isDataIdentical = bytes.Equal(fileDataWithForcedUnixLineBreak, data)
	}

	return isDataIdentical, nil
}
