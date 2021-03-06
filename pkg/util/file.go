package util

import (
	"os"
	"path/filepath"
	"strings"
)

// FileExists returns true if path exists
func FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if isNotExistError(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func RegularFileExists(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		if isNotExistError(err) {
			return false, nil
		}

		return false, err
	}

	return fileInfo.Mode().IsRegular(), nil
}

func DirExists(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		if isNotExistError(err) {
			return false, nil
		}

		return false, err
	}

	return fileInfo.IsDir(), nil
}

func isNotExistError(err error) bool {
	return os.IsNotExist(err) || IsNotADirectoryError(err)
}

func IsNotADirectoryError(err error) bool {
	return strings.HasSuffix(err.Error(), "not a directory")
}

func GetRelativeToBaseFilepath(base, path string) string {
	path = GetAbsoluteFilepath(path)
	base = GetAbsoluteFilepath(base)

	res, err := filepath.Rel(base, path)
	if err != nil {
		panic(err.Error())
	}

	return res
}

func GetAbsoluteFilepath(absOrRelPath string) string {
	absPath, err := filepath.Abs(absOrRelPath)
	if err != nil {
		panic(err.Error())
	}

	return absPath
}

func IsSubpathOfBasePath(basePath, path string) bool {
	basePathParts := SplitFilepath(basePath)
	pathParts := SplitFilepath(path)

	if len(basePathParts) > len(pathParts) {
		return false
	}
	for i := range basePathParts {
		if basePathParts[i] != pathParts[i] {
			return false
		}
	}
	return true
}
