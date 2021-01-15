package dismiss

import (
	"fmt"
	"time"

	"github.com/werf/werf/pkg/deploy/helm/command_helpers"

	"github.com/spf13/cobra"

	cmd_helm "helm.sh/helm/v3/cmd/helm"
	"helm.sh/helm/v3/pkg/action"

	"github.com/werf/kubedog/pkg/kube"
	"github.com/werf/logboek"

	"github.com/werf/werf/cmd/werf/common"
	"github.com/werf/werf/pkg/deploy/helm"
	"github.com/werf/werf/pkg/deploy/helm/chart_extender"
	"github.com/werf/werf/pkg/deploy/lock_manager"
	"github.com/werf/werf/pkg/docker"
	"github.com/werf/werf/pkg/git_repo"
	"github.com/werf/werf/pkg/image"
	"github.com/werf/werf/pkg/tmp_manager"
	"github.com/werf/werf/pkg/true_git"
	"github.com/werf/werf/pkg/werf"
	"github.com/werf/werf/pkg/werf/global_warnings"
)

var cmdData struct {
	WithNamespace bool
	WithHooks     bool
}

var commonCmdData common.CmdData

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dismiss",
		Short: "Delete application from Kubernetes",
		Long: common.GetLongCommandDescription(`Delete application from Kubernetes.

Helm Release will be purged and optionally Kubernetes Namespace.

Environment is a required param for the dismiss by default, because it is needed to construct Helm Release name and Kubernetes Namespace. Either --env or $WERF_ENV should be specified for command.

Read more info about Helm Release name, Kubernetes Namespace and how to change it: https://werf.io/documentation/advanced/helm/basics.html`),
		Example: `  # Dismiss project named 'myproject' previously deployed app from 'dev' environment; helm release name and namespace will be named as 'myproject-dev'
  $ werf dismiss --env dev

  # Dismiss project with namespace
  $ werf dismiss --env my-feature-branch --with-namespace

  # Dismiss project using specified helm release name and namespace
  $ werf dismiss --release myrelease --namespace myns`,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			defer global_warnings.PrintGlobalWarnings(common.BackgroundContext())

			if err := common.ProcessLogOptions(&commonCmdData); err != nil {
				common.PrintHelp(cmd)
				return err
			}
			common.LogVersion()

			return common.LogRunningTime(func() error {
				return runDismiss()
			})
		},
	}

	common.SetupTmpDir(&commonCmdData, cmd)
	common.SetupConfigTemplatesDir(&commonCmdData, cmd)
	common.SetupConfigPath(&commonCmdData, cmd)
	common.SetupEnvironment(&commonCmdData, cmd)

	common.SetupGiterminismInspectorOptions(&commonCmdData, cmd)

	common.SetupHomeDir(&commonCmdData, cmd)
	common.SetupDir(&commonCmdData, cmd)

	common.SetupSecondaryStagesStorageOptions(&commonCmdData, cmd)
	common.SetupStagesStorageOptions(&commonCmdData, cmd)
	common.SetupSynchronization(&commonCmdData, cmd)

	common.SetupRelease(&commonCmdData, cmd)
	common.SetupNamespace(&commonCmdData, cmd)

	common.SetupKubeConfig(&commonCmdData, cmd)
	common.SetupKubeConfigBase64(&commonCmdData, cmd)
	common.SetupKubeContext(&commonCmdData, cmd)

	common.SetupStatusProgressPeriod(&commonCmdData, cmd)
	common.SetupHooksStatusProgressPeriod(&commonCmdData, cmd)
	common.SetupReleasesHistoryMax(&commonCmdData, cmd)

	common.SetupDockerConfig(&commonCmdData, cmd, "")

	common.SetupLogOptions(&commonCmdData, cmd)
	common.SetupLogProjectDir(&commonCmdData, cmd)

	cmd.Flags().BoolVarP(&cmdData.WithNamespace, "with-namespace", "", common.GetBoolEnvironmentDefaultFalse("WERF_WITH_NAMESPACE"), "Delete Kubernetes Namespace after purging Helm Release (default $WERF_WITH_NAMESPACE)")
	cmd.Flags().BoolVarP(&cmdData.WithHooks, "with-hooks", "", common.GetBoolEnvironmentDefaultTrue("WERF_WITH_HOOKS"), "Delete Helm Release hooks getting from existing revisions (default $WERF_WITH_HOOKS or true)")

	return cmd
}

func runDismiss() error {
	tmp_manager.AutoGCEnabled = true
	ctx := common.BackgroundContext()

	if err := werf.Init(*commonCmdData.TmpDir, *commonCmdData.HomeDir); err != nil {
		return fmt.Errorf("initialization error: %s", err)
	}

	if err := common.InitGiterminismInspector(&commonCmdData); err != nil {
		return err
	}

	if err := git_repo.Init(); err != nil {
		return err
	}

	if err := true_git.Init(true_git.Options{LiveGitOutput: *commonCmdData.LogVerbose || *commonCmdData.LogDebug}); err != nil {
		return err
	}

	if err := image.Init(); err != nil {
		return err
	}

	common.LogKubeContext(kube.Context)

	if err := docker.Init(ctx, *commonCmdData.DockerConfig, *commonCmdData.LogVerbose, *commonCmdData.LogDebug); err != nil {
		return err
	}

	ctxWithDockerCli, err := docker.NewContext(ctx)
	if err != nil {
		return err
	}
	ctx = ctxWithDockerCli

	projectDir, err := common.GetProjectDir(&commonCmdData)
	if err != nil {
		return fmt.Errorf("getting project dir failed: %s", err)
	}

	common.ProcessLogProjectDir(&commonCmdData, projectDir)

	giterminismManager, err := common.GetGiterminismManager(&commonCmdData)
	if err != nil {
		return err
	}

	werfConfig, err := common.GetRequiredWerfConfig(ctx, projectDir, &commonCmdData, giterminismManager, common.GetWerfConfigOptions(&commonCmdData, true))
	if err != nil {
		return fmt.Errorf("unable to load werf config: %s", err)
	}
	logboek.LogOptionalLn()

	common.SetupOndemandKubeInitializer(*commonCmdData.KubeContext, *commonCmdData.KubeConfig, *commonCmdData.KubeConfigBase64)
	if err := common.GetOndemandKubeInitializer().Init(ctx); err != nil {
		return err
	}

	releaseName, err := common.GetHelmRelease(*commonCmdData.Release, *commonCmdData.Environment, werfConfig)
	if err != nil {
		return err
	}

	namespace, err := common.GetKubernetesNamespace(*commonCmdData.Namespace, *commonCmdData.Environment, werfConfig)
	if err != nil {
		return err
	}

	var lockManager *lock_manager.LockManager
	if m, err := lock_manager.NewLockManager(namespace); err != nil {
		return fmt.Errorf("unable to create lock manager: %s", err)
	} else {
		lockManager = m
	}

	chartDir, err := common.GetHelmChartDir(projectDir, &commonCmdData, werfConfig)
	if err != nil {
		return fmt.Errorf("getting helm chart dir failed: %s", err)
	}

	wc := chart_extender.NewWerfChart(giterminismManager, nil, projectDir, chartDir, cmd_helm.Settings, chart_extender.WerfChartOptions{})

	if err := wc.SetEnv(*commonCmdData.Environment); err != nil {
		return err
	}
	if err := wc.SetWerfConfig(werfConfig); err != nil {
		return err
	}

	wc.SetChartExtenderContext(ctx)

	actionConfig := new(action.Configuration)
	if err := helm.InitActionConfig(ctx, common.GetOndemandKubeInitializer(), namespace, cmd_helm.Settings, actionConfig, helm.InitActionConfigOptions{
		StatusProgressPeriod:      time.Duration(*commonCmdData.StatusProgressPeriodSeconds) * time.Second,
		HooksStatusProgressPeriod: time.Duration(*commonCmdData.HooksStatusProgressPeriodSeconds) * time.Second,
		KubeConfigOptions: kube.KubeConfigOptions{
			Context:          *commonCmdData.KubeContext,
			ConfigPath:       *commonCmdData.KubeConfig,
			ConfigDataBase64: *commonCmdData.KubeConfigBase64,
		},
		ReleasesHistoryMax: *commonCmdData.ReleasesHistoryMax,
	}); err != nil {
		return err
	}

	helmUninstallCmd := cmd_helm.NewUninstallCmd(actionConfig, logboek.ProxyOutStream(), cmd_helm.UninstallCmdOptions{
		DeleteNamespace: &cmdData.WithNamespace,
		DeleteHooks:     &cmdData.WithHooks,
	})

	return command_helpers.LockReleaseWrapper(ctx, releaseName, lockManager, func() error {
		return helmUninstallCmd.RunE(helmUninstallCmd, []string{releaseName})
	})
}
