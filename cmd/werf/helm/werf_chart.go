package helm

import (
	"context"

	"github.com/werf/werf/pkg/deploy"
	"github.com/werf/werf/pkg/deploy/helm/chart_extender"

	"github.com/spf13/cobra"
	cmd_werf_common "github.com/werf/werf/cmd/werf/common"
)

func SetupRenderRelatedWerfChartParams(cmd *cobra.Command, commonCmdData *cmd_werf_common.CmdData) {
	cmd_werf_common.SetupAddAnnotations(commonCmdData, cmd)
	cmd_werf_common.SetupAddLabels(commonCmdData, cmd)

	cmd_werf_common.SetupSecretValues(commonCmdData, cmd)
	cmd_werf_common.SetupIgnoreSecretKey(commonCmdData, cmd)
}

func InitRenderRelatedWerfChartParams(ctx context.Context, commonCmdData *cmd_werf_common.CmdData, wc *chart_extender.WerfChartStub, chartDir string) error {
	if extraAnnotations, err := cmd_werf_common.GetUserExtraAnnotations(commonCmdData); err != nil {
		return err
	} else {
		wc.AddExtraAnnotationsAndLabels(extraAnnotations, nil)
	}

	if extraLabels, err := cmd_werf_common.GetUserExtraLabels(commonCmdData); err != nil {
		return err
	} else {
		wc.AddExtraAnnotationsAndLabels(nil, extraLabels)
	}

	wc.SetupSecretValueFiles(*commonCmdData.SecretValues)
	// NOTE: project-dir is the same as chart-dir for werf helm install/upgrade commands
	// NOTE: project-dir is werf-project dir only for werf converge/dismiss commands

	// FIXME: localGitRepo should not be needed by deploy secret manager
	if m, err := deploy.GetSafeSecretManager(ctx, chartDir, chartDir, *commonCmdData.SecretValues, nil, *commonCmdData.IgnoreSecretKey); err != nil {
		return err
	} else {
		wc.SetupSecretManager(m)
	}

	return nil
}
