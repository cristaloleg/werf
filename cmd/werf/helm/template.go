package helm

import (
	"fmt"
	"os"

	"github.com/werf/werf/pkg/deploy/helm/chart_extender"

	"github.com/spf13/cobra"
	"github.com/werf/werf/cmd/werf/common"
	cmd_werf_common "github.com/werf/werf/cmd/werf/common"

	"helm.sh/helm/v3/pkg/action"

	cmd_helm "helm.sh/helm/v3/cmd/helm"
)

var templateCmdData cmd_werf_common.CmdData

func NewTemplateCmd(actionConfig *action.Configuration, wc *chart_extender.WerfChartStub) *cobra.Command {
	postRenderer, err := wc.GetPostRenderer()
	if err != nil {
		panic(err.Error())
	}

	cmd, helmAction := cmd_helm.NewTemplateCmd(actionConfig, os.Stdout, cmd_helm.TemplateCmdOptions{
		PostRenderer: postRenderer,
	})
	SetupRenderRelatedWerfChartParams(cmd, &templateCmdData)

	oldRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx := common.BackgroundContext()

		if _, chartDir, err := helmAction.NameAndChart(args); err != nil {
			return err
		} else {
			if err := InitRenderRelatedWerfChartParams(ctx, &templateCmdData, wc, chartDir); err != nil {
				return fmt.Errorf("unable to init werf chart: %s", err)
			}
			return oldRunE(cmd, args)
		}
	}

	return cmd
}
