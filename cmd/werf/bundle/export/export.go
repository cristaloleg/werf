package export

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/werf/werf/pkg/deploy/helm/command_helpers"

	"helm.sh/helm/v3/pkg/getter"

	"github.com/werf/werf/pkg/config"
	"github.com/werf/werf/pkg/git_repo"
	"github.com/werf/werf/pkg/werf/global_warnings"

	"github.com/werf/werf/pkg/deploy/helm"

	"github.com/werf/werf/pkg/deploy"
	"github.com/werf/werf/pkg/deploy/secret"

	"github.com/werf/werf/pkg/deploy/werf_chart"
	cmd_helm "helm.sh/helm/v3/cmd/helm"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli/values"

	"github.com/spf13/cobra"

	"github.com/werf/logboek"
	"github.com/werf/logboek/pkg/level"

	"github.com/werf/werf/cmd/werf/common"
	"github.com/werf/werf/pkg/build"
	"github.com/werf/werf/pkg/container_runtime"
	"github.com/werf/werf/pkg/docker"
	"github.com/werf/werf/pkg/image"
	"github.com/werf/werf/pkg/ssh_agent"
	"github.com/werf/werf/pkg/storage/manager"
	"github.com/werf/werf/pkg/tmp_manager"
	"github.com/werf/werf/pkg/true_git"
	"github.com/werf/werf/pkg/werf"
)

var cmdData struct {
	Destination string
}

var commonCmdData common.CmdData

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "export",
		Short:                 "Export bundle",
		Long:                  common.GetLongCommandDescription(`Export bundle into the provided directory (or into directory named as a resulting chart in the current working directory). Werf bundle contains built images defined in the werf.yaml, helm chart, service values which contain built images tags, any custom values and set values params provided during publish invocation, werf addon templates (like werf_image).`),
		DisableFlagsInUseLine: true,
		Annotations: map[string]string{
			common.CmdEnvAnno: common.EnvsDescription(common.WerfDebugAnsibleArgs, common.WerfSecretKey),
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			defer global_warnings.PrintGlobalWarnings(common.BackgroundContext())

			logboek.Streams().Mute()
			logboek.SetAcceptedLevel(level.Error)

			if err := common.ProcessLogOptionsDefaultQuiet(&commonCmdData); err != nil {
				common.PrintHelp(cmd)
				return err
			}

			common.LogVersion()

			return common.LogRunningTime(func() error {
				return runExport()
			})
		},
	}

	common.SetupDir(&commonCmdData, cmd)
	common.SetupGiterminismInspectorOptions(&commonCmdData, cmd)
	common.SetupConfigTemplatesDir(&commonCmdData, cmd)
	common.SetupConfigPath(&commonCmdData, cmd)
	common.SetupEnvironment(&commonCmdData, cmd)

	common.SetupTmpDir(&commonCmdData, cmd)
	common.SetupHomeDir(&commonCmdData, cmd)
	common.SetupSSHKey(&commonCmdData, cmd)

	common.SetupIntrospectAfterError(&commonCmdData, cmd)
	common.SetupIntrospectBeforeError(&commonCmdData, cmd)
	common.SetupIntrospectStage(&commonCmdData, cmd)

	common.SetupSecondaryStagesStorageOptions(&commonCmdData, cmd)
	common.SetupStagesStorageOptions(&commonCmdData, cmd)

	common.SetupDockerConfig(&commonCmdData, cmd, "Command needs granted permissions to read, pull and push images into the specified repo and to pull base images")
	common.SetupInsecureRegistry(&commonCmdData, cmd)
	common.SetupSkipTlsVerifyRegistry(&commonCmdData, cmd)

	common.SetupLogOptions(&commonCmdData, cmd)
	common.SetupLogProjectDir(&commonCmdData, cmd)

	common.SetupSynchronization(&commonCmdData, cmd)

	common.SetupAddAnnotations(&commonCmdData, cmd)
	common.SetupAddLabels(&commonCmdData, cmd)

	common.SetupSet(&commonCmdData, cmd)
	common.SetupSetString(&commonCmdData, cmd)
	common.SetupSetFile(&commonCmdData, cmd)
	common.SetupValues(&commonCmdData, cmd)
	common.SetupSecretValues(&commonCmdData, cmd)
	common.SetupIgnoreSecretKey(&commonCmdData, cmd)

	common.SetupReportPath(&commonCmdData, cmd)
	common.SetupReportFormat(&commonCmdData, cmd)

	common.SetupVirtualMerge(&commonCmdData, cmd)
	common.SetupVirtualMergeFromCommit(&commonCmdData, cmd)
	common.SetupVirtualMergeIntoCommit(&commonCmdData, cmd)

	common.SetupParallelOptions(&commonCmdData, cmd, common.DefaultBuildParallelTasksLimit)

	common.SetupSkipBuild(&commonCmdData, cmd)

	cmd.Flags().StringVarP(&cmdData.Destination, "destination", "d", os.Getenv("WERF_DESTINATION"), "Export bundle into the provided directory ($WERF_DESTINATION or chart-name by default)")

	return cmd
}

func runExport() error {
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

	if err := image.Init(); err != nil {
		return err
	}

	if err := true_git.Init(true_git.Options{LiveGitOutput: *commonCmdData.LogVerbose || *commonCmdData.LogDebug}); err != nil {
		return err
	}

	if err := common.DockerRegistryInit(&commonCmdData); err != nil {
		return err
	}

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

	localGitRepo, err := common.OpenLocalGitRepo(projectDir)
	if err != nil {
		return fmt.Errorf("unable to open local repo %s: %s", projectDir, err)
	}

	giterminismManager, err := common.GetGiterminismManager(&commonCmdData)
	if err != nil {
		return err
	}

	werfConfig, err := common.GetRequiredWerfConfig(ctx, projectDir, &commonCmdData, giterminismManager, config.WerfConfigOptions{LogRenderedFilePath: true, Env: *commonCmdData.Environment})
	if err != nil {
		return fmt.Errorf("unable to load werf config: %s", err)
	}

	projectName := werfConfig.Meta.Project

	chartDir, err := common.GetHelmChartDir(projectDir, &commonCmdData, werfConfig)
	if err != nil {
		return fmt.Errorf("getting helm chart dir failed: %s", err)
	}

	projectTmpDir, err := tmp_manager.CreateProjectDir(ctx)
	if err != nil {
		return fmt.Errorf("getting project tmp dir failed: %s", err)
	}
	defer tmp_manager.ReleaseProjectDir(projectTmpDir)

	if err := ssh_agent.Init(ctx, *commonCmdData.SSHKeys); err != nil {
		return fmt.Errorf("cannot initialize ssh agent: %s", err)
	}
	defer func() {
		err := ssh_agent.Terminate()
		if err != nil {
			logboek.Warn().LogF("WARNING: ssh agent termination failed: %s\n", err)
		}
	}()

	userExtraAnnotations, err := common.GetUserExtraAnnotations(&commonCmdData)
	if err != nil {
		return err
	}

	userExtraLabels, err := common.GetUserExtraLabels(&commonCmdData)
	if err != nil {
		return err
	}

	buildOptions, err := common.GetBuildOptions(&commonCmdData, werfConfig)
	if err != nil {
		return err
	}

	logboek.LogOptionalLn()

	repoAddress, err := common.GetStagesStorageAddress(&commonCmdData)
	if err != nil {
		return err
	}

	var imagesInfoGetters []*image.InfoGetter
	var imagesRepository string

	if len(werfConfig.StapelImages) != 0 || len(werfConfig.ImagesFromDockerfile) != 0 {
		containerRuntime := &container_runtime.LocalDockerServerRuntime{} // TODO
		stagesStorage, err := common.GetStagesStorage(repoAddress, containerRuntime, &commonCmdData)
		if err != nil {
			return err
		}
		synchronization, err := common.GetSynchronization(ctx, &commonCmdData, projectName, stagesStorage)
		if err != nil {
			return err
		}
		stagesStorageCache, err := common.GetStagesStorageCache(synchronization)
		if err != nil {
			return err
		}
		storageLockManager, err := common.GetStorageLockManager(ctx, synchronization)
		if err != nil {
			return err
		}
		secondaryStagesStorageList, err := common.GetSecondaryStagesStorageList(stagesStorage, containerRuntime, &commonCmdData)
		if err != nil {
			return err
		}

		storageManager := manager.NewStorageManager(projectName, stagesStorage, secondaryStagesStorageList, storageLockManager, stagesStorageCache)

		imagesRepository = storageManager.StagesStorage.String()

		conveyorOptions, err := common.GetConveyorOptionsWithParallel(&commonCmdData, buildOptions)
		if err != nil {
			return err
		}

		conveyorWithRetry := build.NewConveyorWithRetryWrapper(werfConfig, giterminismManager, localGitRepo, nil, projectDir, projectTmpDir, ssh_agent.SSHAuthSock, containerRuntime, storageManager, storageLockManager, conveyorOptions)
		defer conveyorWithRetry.Terminate()

		if err := conveyorWithRetry.WithRetryBlock(ctx, func(c *build.Conveyor) error {
			if *commonCmdData.SkipBuild {
				if err := c.ShouldBeBuilt(ctx); err != nil {
					return err
				}
			} else {
				if err := c.Build(ctx, buildOptions); err != nil {
					return err
				}
			}

			imagesInfoGetters = c.GetImageInfoGetters()

			return nil
		}); err != nil {
			return err
		}

		logboek.LogOptionalLn()
	}

	var secretsManager secret.Manager
	if m, err := deploy.GetSafeSecretManager(ctx, projectDir, chartDir, *commonCmdData.SecretValues, &localGitRepo, *commonCmdData.IgnoreSecretKey); err != nil {
		return err
	} else {
		secretsManager = m
	}

	wc := werf_chart.NewWerfChart(ctx, &localGitRepo, projectDir, werf_chart.WerfChartOptions{
		ReleaseName: "RELEASE",
		ChartDir:    chartDir,

		SecretValueFiles: *commonCmdData.SecretValues,
		ExtraAnnotations: userExtraAnnotations,
		ExtraLabels:      userExtraLabels,

		SecretsManager: secretsManager,
	})
	if err := wc.SetEnv(*commonCmdData.Environment); err != nil {
		return err
	}
	if err := wc.SetWerfConfig(werfConfig); err != nil {
		return err
	}
	if vals, err := werf_chart.GetServiceValues(ctx, werfConfig.Meta.Project, imagesRepository, imagesInfoGetters, werf_chart.ServiceValuesOptions{Env: *commonCmdData.Environment}); err != nil {
		return fmt.Errorf("error creating service values: %s", err)
	} else if err := wc.SetServiceValues(vals); err != nil {
		return err
	}

	actionConfig := new(action.Configuration)
	if err := helm.InitActionConfig(ctx, nil, "", cmd_helm.Settings, actionConfig, helm.InitActionConfigOptions{}); err != nil {
		return err
	}

	cmd_helm.Settings.Debug = *commonCmdData.LogDebug

	loader.GlobalLoadOptions = &loader.LoadOptions{
		ChartExtender: wc,
		SubchartExtenderFactoryFunc: func() chart.ChartExtender {
			return werf_chart.NewWerfChart(ctx, nil, projectDir, werf_chart.WerfChartOptions{})
		},
		LoadDirFunc:     common.MakeChartDirLoadFunc(ctx, localGitRepo, projectDir, cmd_helm.Settings, command_helpers.BuildChartDependenciesOptions{}),
		LocateChartFunc: common.MakeLocateChartFunc(ctx, localGitRepo, projectDir),
		ReadFileFunc:    common.MakeHelmReadFileFunc(ctx, localGitRepo, projectDir),
	}

	valueOpts := &values.Options{
		ValueFiles:   *commonCmdData.Values,
		StringValues: *commonCmdData.SetString,
		Values:       *commonCmdData.Set,
		FileValues:   *commonCmdData.SetFile,
	}

	helmTemplateCmd, _ := cmd_helm.NewTemplateCmd(actionConfig, ioutil.Discard, cmd_helm.TemplateCmdOptions{
		PostRenderer: wc.ExtraAnnotationsAndLabelsPostRenderer,
		ValueOpts:    valueOpts,
	})
	if err := wc.WrapTemplate(ctx, func() error {
		return helmTemplateCmd.RunE(helmTemplateCmd, []string{"RELEASE", chartDir})
	}); err != nil {
		return err
	}

	destinationDir := cmdData.Destination
	if destinationDir == "" {
		destinationDir = wc.HelmChart.Metadata.Name
	}

	p := getter.All(cmd_helm.Settings)
	if vals, err := valueOpts.MergeValues(p, loader.GlobalLoadOptions.ReadFileFunc); err != nil {
		return err
	} else if _, err := wc.CreateNewBundle(ctx, destinationDir, vals); err != nil {
		return fmt.Errorf("unable to create bundle: %s", err)
	}

	return nil
}
