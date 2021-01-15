package chart_extender

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/werf/werf/pkg/git_repo"

	"github.com/werf/werf/pkg/deploy/secret"
	"sigs.k8s.io/yaml"
)

func DecodeSecretValuesFileFromGitCommit(ctx context.Context, path string, commit string, localGitRepo *git_repo.Local, m secret.Manager, projectDir string) (map[string]interface{}, error) {
	var data []byte

	if d, err := git_repo.ReadCommitFileAndCompareWithProjectFile(ctx, *localGitRepo, commit, projectDir, path); err != nil {
		return nil, err
	} else {
		data = d
	}

	decodedData, err := m.DecryptYamlData(data)
	if err != nil {
		return nil, fmt.Errorf("cannot decode file %q secret data: %s", path, err)
	}

	rawValues := map[string]interface{}{}
	if err := yaml.Unmarshal(decodedData, &rawValues); err != nil {
		return nil, fmt.Errorf("cannot unmarshal secret values file %s: %s", path, err)
	}

	return rawValues, nil
}

func DecodeSecretValuesFileFromFilesystem(ctx context.Context, path string, m secret.Manager) (map[string]interface{}, error) {
	var data []byte

	if d, err := ioutil.ReadFile(path); err != nil {
		return nil, fmt.Errorf("cannot read file %q: %s", path, err)
	} else {
		data = d
	}

	decodedData, err := m.DecryptYamlData(data)
	if err != nil {
		return nil, fmt.Errorf("cannot decode file %q secret data: %s", path, err)
	}

	rawValues := map[string]interface{}{}
	if err := yaml.Unmarshal(decodedData, &rawValues); err != nil {
		return nil, fmt.Errorf("cannot unmarshal secret values file %s: %s", path, err)
	}

	return rawValues, nil
}
