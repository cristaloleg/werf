package deploy

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/werf/logboek"

	"github.com/werf/werf/pkg/deploy/helm/chart_extender"
	"github.com/werf/werf/pkg/deploy/secret"
	"github.com/werf/werf/pkg/git_repo"
	"github.com/werf/werf/pkg/giterminism_inspector"
	"github.com/werf/werf/pkg/util"
)

func GetSafeSecretManager(ctx context.Context, projectDir, helmChartDir string, secretValues []string, localGitRepo *git_repo.Local, ignoreSecretKey bool) (secret.Manager, error) {
	isSecretsExists := false

	secretDirPath := filepath.Join(helmChartDir, chart_extender.SecretDirName)
	defaultSecretValuesFilePath := filepath.Join(helmChartDir, chart_extender.DefaultSecretValuesFileName)
	if giterminism_inspector.LooseGiterminism || localGitRepo == nil {
		if exists, err := util.DirExists(secretDirPath); err != nil {
			return nil, fmt.Errorf("unable to check directory %s existence: %s", secretDirPath, err)
		} else if exists {
			isSecretsExists = true
		}

		if exists, err := util.RegularFileExists(defaultSecretValuesFilePath); err != nil {
			return nil, fmt.Errorf("unable to check file %s existence: %s", defaultSecretValuesFilePath, err)
		} else if exists {
			isSecretsExists = true
		}
	} else {
		commit, err := localGitRepo.HeadCommit(ctx)
		if err != nil {
			return nil, fmt.Errorf("unable to get local repo head commit: %s", err)
		}

		if exists, err := localGitRepo.IsCommitDirectoryExists(ctx, secretDirPath, commit); err != nil {
			return nil, fmt.Errorf("error checking existence of the directory %q in the local git repo commit %s: %s", secretDirPath, commit, err)
		} else if exists {
			isSecretsExists = true
		}

		if exists, err := localGitRepo.IsCommitFileExists(ctx, commit, defaultSecretValuesFilePath); err != nil {
			return nil, fmt.Errorf("error checking existence of the file %q in the local git repo commit %s: %s", defaultSecretValuesFilePath, commit, err)
		} else if exists {
			isSecretsExists = true
		}
	}

	if len(secretValues) > 0 {
		isSecretsExists = true
	}

	if isSecretsExists {
		if ignoreSecretKey {
			logboek.Context(ctx).Default().LogLnDetails("Secrets decryption disabled")
			return secret.NewSafeManager()
		}

		key, err := secret.GetSecretKey(projectDir)
		if err != nil {
			return nil, err
		}

		return secret.NewManager(key)
	} else {
		return secret.NewSafeManager()
	}
}
