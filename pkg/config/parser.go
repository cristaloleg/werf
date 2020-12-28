package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/bmatcuk/doublestar"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"gopkg.in/yaml.v2"

	"github.com/werf/logboek"

	"github.com/werf/werf/pkg/git_repo"
	"github.com/werf/werf/pkg/giterminism_inspector"
	"github.com/werf/werf/pkg/logging"
	"github.com/werf/werf/pkg/slug"
	"github.com/werf/werf/pkg/tmp_manager"
	"github.com/werf/werf/pkg/util"
)

type WerfConfigOptions struct {
	LogRenderedFilePath bool
	Env                 string
}

func RenderWerfConfig(ctx context.Context, projectDir, relWerfConfigPath, relWerfConfigTemplatesDir string, imagesToProcess []string, localGitRepo git_repo.Local, opts WerfConfigOptions) error {
	werfConfig, err := GetWerfConfig(ctx, projectDir, relWerfConfigPath, relWerfConfigTemplatesDir, localGitRepo, opts)
	if err != nil {
		return err
	}

	if len(imagesToProcess) == 0 {
		werfConfigRenderContent, err := renderWerfConfigYaml(ctx, projectDir, relWerfConfigPath, relWerfConfigTemplatesDir, localGitRepo, opts.Env)
		if err != nil {
			return fmt.Errorf("cannot parse config: %s", err)
		}

		fmt.Print(werfConfigRenderContent)
	} else {
		var imageDocs []string

		for _, imageToProcess := range imagesToProcess {
			if !werfConfig.HasImageOrArtifact(imageToProcess) {
				return fmt.Errorf("specified image %s is not defined in werf.yaml", logging.ImageLogName(imageToProcess, false))
			} else {
				if i := werfConfig.GetArtifact(imageToProcess); i != nil {
					imageDocs = append(imageDocs, string(i.raw.doc.Content))
				} else if i := werfConfig.GetStapelImage(imageToProcess); i != nil {
					imageDocs = append(imageDocs, string(i.raw.doc.Content))
				} else if i := werfConfig.GetDockerfileImage(imageToProcess); i != nil {
					imageDocs = append(imageDocs, string(i.raw.doc.Content))
				}
			}
		}

		fmt.Print(strings.Join(imageDocs, "---\n"))
	}

	return nil
}

func GetWerfConfig(ctx context.Context, projectDir, relWerfConfigPath, relWerfConfigTemplatesDir string, localGitRepo git_repo.Local, opts WerfConfigOptions) (*WerfConfig, error) {
	werfConfigRenderContent, err := renderWerfConfigYaml(ctx, projectDir, relWerfConfigPath, relWerfConfigTemplatesDir, localGitRepo, opts.Env)
	if err != nil {
		return nil, fmt.Errorf("cannot parse config: %s", err)
	}

	werfConfigRenderPath, err := tmp_manager.CreateWerfConfigRender(ctx)
	if err != nil {
		return nil, err
	}

	if opts.LogRenderedFilePath {
		logboek.Context(ctx).LogF("Using werf config render file: %s\n", werfConfigRenderPath)
	}

	err = writeWerfConfigRender(werfConfigRenderContent, werfConfigRenderPath)
	if err != nil {
		return nil, fmt.Errorf("unable to write rendered config to %s: %s", werfConfigRenderPath, err)
	}

	docs, err := splitByDocs(werfConfigRenderContent, werfConfigRenderPath)
	if err != nil {
		return nil, err
	}

	meta, rawStapelImages, rawImagesFromDockerfile, err := splitByMetaAndRawImages(docs)
	if err != nil {
		return nil, err
	}

	if meta == nil {
		defaultProjectName, err := GetProjectName(ctx, projectDir)
		if err != nil {
			return nil, fmt.Errorf("failed to get default project name: %s", err)
		}

		format := "meta config section (part of YAML stream separated by three hyphens, https://yaml.org/spec/1.2/spec.html#id2800132) is not defined: add following example config section with required fields, e.g:\n\n" +
			"```\n" +
			"configVersion: 1\n" +
			"project: %s\n" +
			"---\n" +
			"```\n\n" +
			"##############################################################################################################################\n" +
			"###           WARNING! Project name cannot be changed later without rebuilding and redeploying your application!           ###\n" +
			"###       Project name should be unique within group of projects that shares build hosts and deployed into the same        ###\n" +
			"###                    Kubernetes clusters (i.e. unique across all groups within the same gitlab).                         ###\n" +
			"###              Read more about meta config section: https://werf.io/documentation/reference/werf_yaml.html               ###\n" +
			"##############################################################################################################################"

		return nil, fmt.Errorf(format, defaultProjectName)
	}

	werfConfig, err := prepareWerfConfig(rawStapelImages, rawImagesFromDockerfile, meta)
	if err != nil {
		return nil, err
	}

	return werfConfig, nil
}

func GetProjectName(ctx context.Context, projectDir string) (string, error) {
	name := filepath.Base(projectDir)

	if exist, err := util.DirExists(filepath.Join(projectDir, ".git")); err != nil {
		return "", err
	} else if exist {
		remoteOriginUrl, err := gitOwnRepoOriginUrl(ctx, projectDir)
		if err != nil {
			return "", err
		}

		if remoteOriginUrl != "" {
			ep, err := transport.NewEndpoint(remoteOriginUrl)
			if err != nil {
				return "", fmt.Errorf("bad url '%s': %s", remoteOriginUrl, err)
			}

			gitName := strings.TrimSuffix(ep.Path, ".git")

			return slug.Project(gitName), nil
		}
	}

	return slug.Project(name), nil
}

func gitOwnRepoOriginUrl(ctx context.Context, projectDir string) (string, error) {
	localGitRepo := &git_repo.Local{
		Path:   projectDir,
		GitDir: filepath.Join(projectDir, ".git"),
	}

	remoteOriginUrl, err := localGitRepo.RemoteOriginUrl(ctx)
	if err != nil {
		return "", nil
	}

	return remoteOriginUrl, nil
}

func writeWerfConfigRender(werfConfigRenderContent string, werfConfigRenderPath string) error {
	werfConfigRenderFile, err := os.OpenFile(werfConfigRenderPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	_, err = werfConfigRenderFile.Write([]byte(werfConfigRenderContent))
	if err != nil {
		return err
	}

	err = werfConfigRenderFile.Close()
	if err != nil {
		return err
	}

	return nil
}

func splitByDocs(werfConfigRenderContent string, werfConfigRenderPath string) ([]*doc, error) {
	var docs []*doc
	var line int
	for _, docContent := range splitContent([]byte(werfConfigRenderContent)) {
		if !emptyDocContent(docContent) {
			docs = append(docs, &doc{
				Line:           line,
				Content:        docContent,
				RenderFilePath: werfConfigRenderPath,
			})
		}

		contentLines := bytes.Split(docContent, []byte("\n"))
		if string(contentLines[len(contentLines)-1]) == "" {
			contentLines = contentLines[0 : len(contentLines)-1]
		}
		line += len(contentLines) + 1
	}

	return docs, nil
}

func renderWerfConfigYaml(ctx context.Context, projectDir, relWerfConfigPath, relWerfConfigTemplatesDir string, localGitRepo git_repo.Local, env string) (string, error) {
	var commit string
	if c, err := localGitRepo.HeadCommit(ctx); err != nil {
		return "", fmt.Errorf("unable to get local repo head commit: %s", err)
	} else {
		commit = c
	}

	tmpl := template.New("werfConfig")
	tmpl.Funcs(funcMap(tmpl))

	if err := parseWerfConfigTemplatesDir(ctx, tmpl, localGitRepo, commit, projectDir, relWerfConfigTemplatesDir); err != nil {
		return "", err
	}

	if err := parseWerfConfig(ctx, tmpl, localGitRepo, commit, projectDir, relWerfConfigPath); err != nil {
		return "", err
	}

	templateData := make(map[string]interface{})
	templateData["Files"] = files{ctx: ctx, ProjectDir: projectDir, Commit: commit, LocalGitRepo: localGitRepo}
	templateData["Env"] = env

	config, err := executeTemplate(tmpl, "werfConfig", templateData)

	return config, err
}

func parseWerfConfig(ctx context.Context, tmpl *template.Template, localGitRepo git_repo.Local, commit string, projectDir string, relWerfConfigPath string) (err error) {
	var configData []byte
	if giterminism_inspector.LooseGiterminism || giterminism_inspector.IsUncommittedConfigAccepted() {
		configData, err = ioutil.ReadFile(filepath.Join(projectDir, relWerfConfigPath))
		if err != nil {
			return err
		}
	} else {
		configData, err = git_repo.ReadCommitFileAndCompareWithProjectFile(ctx, localGitRepo, commit, projectDir, relWerfConfigPath)
		if err != nil {
			return err
		}
	}

	if _, err := tmpl.Parse(string(configData)); err != nil {
		return err
	}

	return nil
}

func parseWerfConfigTemplatesDir(ctx context.Context, tmpl *template.Template, localGitRepo git_repo.Local, commit string, projectDir string, relWerfConfigTemplatesDir string) error {
	templateNameFunc := func(relTemplatePath string) string {
		return filepath.ToSlash(util.GetRelativeToBaseFilepath(relWerfConfigTemplatesDir, relTemplatePath))
	}

	addTemplatesFromFSFunc := func(relTemplatePath string) error {
		d, err := ioutil.ReadFile(filepath.Join(projectDir, relTemplatePath))
		if err != nil {
			return err
		}

		return addTemplate(tmpl, templateNameFunc(relTemplatePath), string(d))
	}

	addTemplatesFromLocalGitRepoFunc := func(relTemplatePath string) error {
		d, err := git_repo.ReadCommitFileAndCompareWithProjectFile(ctx, localGitRepo, commit, projectDir, relTemplatePath)
		if err != nil {
			return err
		}

		templateName, err := filepath.Rel(relWerfConfigTemplatesDir, relTemplatePath)
		if err != nil {
			return err
		}

		return addTemplate(tmpl, templateName, string(d))
	}

	fsTemplatePathList, err := getWerfConfigTemplatesFromFilesystem(projectDir, relWerfConfigTemplatesDir)
	if err != nil {
		return err
	}

	if giterminism_inspector.LooseGiterminism {
		for _, relPath := range fsTemplatePathList {
			if err := addTemplatesFromFSFunc(relPath); err != nil {
				return err
			}
		}

		return nil
	}

	commitTemplatePathList, err := getWerfConfigTemplatesLocalGitRepo(ctx, localGitRepo, commit, relWerfConfigTemplatesDir)
	if err != nil {
		return err
	}

	for _, relPath := range commitTemplatePathList {
		if accepted, err := giterminism_inspector.IsUncommittedConfigTemplateFileAccepted(relPath); err != nil {
			return err
		} else if accepted {
			continue
		}

		if err := addTemplatesFromLocalGitRepoFunc(relPath); err != nil {
			return err
		}
	}

	if giterminism_inspector.HaveUncommittedConfigTemplates() {
		for _, relPath := range fsTemplatePathList {
			accepted, err := giterminism_inspector.IsUncommittedConfigTemplateFileAccepted(relPath)
			if err != nil {
				return err
			}

			if !accepted {
				continue
			}

			if err := addTemplatesFromFSFunc(relPath); err != nil {
				return err
			}
		}
	} else {
		var commitTemplatePathListToFilepath []string
		for _, path := range commitTemplatePathList {
			commitTemplatePathListToFilepath = append(commitTemplatePathListToFilepath, filepath.FromSlash(path))
		}

		untrackedFiles := util.ExcludeFromStringArray(fsTemplatePathList, commitTemplatePathListToFilepath...)
		for _, path := range untrackedFiles {
			if err := giterminism_inspector.ReportUntrackedConfigTemplateFile(ctx, path); err != nil {
				return err
			}
		}
	}

	return nil
}

func addTemplate(tmpl *template.Template, templateName string, templateContent string) error {
	extraTemplate := tmpl.New(templateName)
	_, err := extraTemplate.Parse(templateContent)
	return err
}

func getWerfConfigTemplatesLocalGitRepo(ctx context.Context, localGitRepo git_repo.Local, commit string, relConfigTemplatesDir string) ([]string, error) {
	paths, err := localGitRepo.GetCommitFilePathList(ctx, commit)
	if err != nil {
		return nil, fmt.Errorf("unable to get files list from local git repo: %s", err)
	}

	var templatesPathList []string
	for _, relPath := range paths {
		if !util.IsSubpathOfBasePath(relConfigTemplatesDir, relPath) {
			continue
		}

		templatesPathList = append(templatesPathList, relPath)
	}

	return templatesPathList, nil
}

func getWerfConfigTemplatesFromFilesystem(projectDir, relWerfConfigTemplatesDir string) ([]string, error) {
	werfConfigTemplatesDir := filepath.Join(projectDir, relWerfConfigTemplatesDir)

	if exist, err := util.DirExists(werfConfigTemplatesDir); err != nil {
		return nil, fmt.Errorf("unable to check existence of directory %s: %s", werfConfigTemplatesDir, err)
	} else if !exist {
		return nil, nil
	}

	var templates []string
	if err := filepath.Walk(werfConfigTemplatesDir, func(fp string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		matched, err := filepath.Match("*.tmpl", fi.Name())
		if err != nil {
			return err
		}

		if matched {
			relToWerfConfigTemplatesDir := util.GetRelativeToBaseFilepath(werfConfigTemplatesDir, fp)
			relToProjectDir := filepath.Join(relWerfConfigTemplatesDir, relToWerfConfigTemplatesDir)
			templates = append(templates, relToProjectDir)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return templates, nil
}

func funcMap(tmpl *template.Template) template.FuncMap {
	funcMap := sprig.TxtFuncMap()
	delete(funcMap, "expandenv")

	funcMap["include"] = func(name string, data interface{}) (string, error) {
		return executeTemplate(tmpl, name, data)
	}
	funcMap["tpl"] = func(templateContent string, data interface{}) (string, error) {
		templateName := util.GenerateConsistentRandomString(10)
		if err := addTemplate(tmpl, templateName, templateContent); err != nil {
			return "", err
		}

		return executeTemplate(tmpl, templateName, data)
	}

	envFunc := funcMap["env"].(func(string) string)
	funcMap["env"] = func(value interface{}) (string, error) {
		envName := fmt.Sprint(value)

		if !giterminism_inspector.LooseGiterminism {
			if err := giterminism_inspector.ReportConfigGoTemplateRenderingEnv(context.Background(), envName); err != nil {
				return "", err
			}
		}

		return envFunc(envName), nil
	}

	return funcMap
}

func executeTemplate(tmpl *template.Template, name string, data interface{}) (string, error) {
	buf := bytes.NewBuffer(nil)
	if err := tmpl.ExecuteTemplate(buf, name, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type files struct {
	ctx          context.Context
	ProjectDir   string
	LocalGitRepo git_repo.Local
	Commit       string
}

func (f files) doGet(path string) (string, error) {
	accepted, err := giterminism_inspector.IsUncommittedConfigGoTemplateRenderingFileAccepted(path)
	if err != nil {
		return "", err
	}

	if giterminism_inspector.LooseGiterminism || accepted {
		filePath := filepath.Join(f.ProjectDir, filepath.FromSlash(path))

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return "", fmt.Errorf("config {{ .Files.Get '%s' }}: file not exist", path)
		} else if err != nil {
			return "", fmt.Errorf("error accessing %s: %s", filePath, err)
		}

		if b, err := ioutil.ReadFile(filePath); err != nil {
			return "", fmt.Errorf("error reading %s: %s", filePath, err)
		} else {
			return string(b), nil
		}
	}

	if exists, err := f.LocalGitRepo.IsCommitFileExists(f.ctx, f.Commit, path); err != nil {
		return "", fmt.Errorf("unable to check existence of %s in the local git repo commit %s: %s", path, f.Commit, err)
	} else if !exists {
		return "", fmt.Errorf("config {{ .Files.Get '%s' }}: file not exist", path)
	}

	if b, err := git_repo.ReadCommitFileAndCompareWithProjectFile(f.ctx, f.LocalGitRepo, f.Commit, f.ProjectDir, path); err != nil {
		return "", fmt.Errorf("error reading %s from local git repo commit %s: %s", path, f.Commit, err)
	} else {
		return string(b), nil
	}
}

func (f files) Get(path string) string {
	if res, err := f.doGet(path); err != nil {
		panic(err.Error())
	} else {
		return res
	}
}

func (f files) doGlobFromFS(pattern string) (map[string]interface{}, error) {
	result := map[string]interface{}{}
	err := util.WalkByPattern(f.ProjectDir, pattern, func(path string, s os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if s.IsDir() {
			return nil
		}

		var filePath string
		if s.Mode()&os.ModeSymlink == os.ModeSymlink {
			link, err := filepath.EvalSymlinks(path)
			if err != nil {
				return fmt.Errorf("eval symlink %s failed: %s", path, err)
			}

			linkStat, err := os.Lstat(link)
			if err != nil {
				return fmt.Errorf("lstat %s failed: %s", linkStat, err)
			}

			if linkStat.IsDir() {
				return nil
			}

			filePath = link
		} else {
			filePath = path
		}

		b, err := ioutil.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read file %s failed: %s", filePath, err)
		}

		resultPath := strings.TrimPrefix(path, f.ProjectDir+string(os.PathSeparator))
		resultPath = filepath.ToSlash(resultPath)
		result[resultPath] = string(b)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (f files) doGlob(ctx context.Context, pattern string) (map[string]interface{}, error) {
	res := map[string]interface{}{}
	var err error
	if giterminism_inspector.LooseGiterminism {
		res, err = f.doGlobFromFS(pattern)
	} else {
		commitPathList, err := f.LocalGitRepo.GetCommitFilePathList(ctx, f.Commit)
		if err != nil {
			return nil, fmt.Errorf("unable to get files list from local git repo: %s", err)
		}

		for _, relFilepath := range commitPathList {
			relPath := filepath.ToSlash(relFilepath)
			if matched, err := doublestar.Match(pattern, relPath); err != nil {
				return nil, err
			} else if !matched {
				continue
			}

			accepted, err := giterminism_inspector.IsUncommittedConfigGoTemplateRenderingFileAccepted(relPath)
			if err != nil {
				return nil, err
			}

			if accepted {
				continue
			}

			data, err := git_repo.ReadCommitFileAndCompareWithProjectFile(ctx, f.LocalGitRepo, f.Commit, f.ProjectDir, relPath)
			if err != nil {
				return nil, err
			}

			res[relPath] = string(data)
		}

		fsPathList, err := f.doGlobFromFS(pattern)
		if err != nil {
			return nil, err
		}
		for relPath, data := range fsPathList {
			accepted, err := giterminism_inspector.IsUncommittedConfigGoTemplateRenderingFileAccepted(relPath)
			if err != nil {
				return nil, err
			}

			if !accepted {
				_, exist := res[relPath]
				if !exist {
					if err := giterminism_inspector.ReportUntrackedConfigGoTemplateRenderingFile(ctx, relPath); err != nil {
						return nil, err
					}
				}

				continue
			}

			res[relPath] = data
		}
	}

	if err != nil {
		return nil, err
	}

	if len(res) == 0 {
		logboek.Context(f.ctx).Warn().LogF("WARNING: No matches found for {{ .Files.Glob '%s' }}\n", pattern)
	}

	return res, nil
}

// Glob returns the hash of regular files and their contents for the paths that are matched pattern
// This function follows only symlinks pointed to a regular file (not to a directory)
func (f files) Glob(pattern string) map[string]interface{} {
	if res, err := f.doGlob(f.ctx, pattern); err != nil {
		panic(err.Error())
	} else {
		return res
	}
}

func splitContent(content []byte) (docsContents [][]byte) {
	const (
		stateLineBegin   = "stateLineBegin"
		stateRegularLine = "stateRegularLine"
		stateDocDash1    = "stateDocDash1"
		stateDocDash2    = "stateDocDash2"
		stateDocDash3    = "stateDocDash3"
		stateDocSpaces   = "stateDocSpaces"
		stateDocComment  = "stateDocComment"
	)

	state := stateLineBegin
	var docStartIndex, separatorLength int
	var docContent []byte
	var index int
	var ch byte
	for index, ch = range content {
		switch ch {
		case '-':
			switch state {
			case stateLineBegin:
				separatorLength = 1
				state = stateDocDash1
			case stateDocDash1, stateDocDash2:
				separatorLength += 1

				switch state {
				case stateDocDash1:
					state = stateDocDash2
				case stateDocDash2:
					state = stateDocDash3
				}
			default:
				state = stateRegularLine
			}
		case '\n':
			switch state {
			case stateDocDash3, stateDocSpaces, stateDocComment:
				if docStartIndex == index-separatorLength {
					docContent = []byte{}
				} else {
					docContent = content[docStartIndex : index-separatorLength]
				}
				docsContents = append(docsContents, docContent)
				docStartIndex = index + 1
			}
			separatorLength = 0
			state = stateLineBegin
		case ' ', '\r', '\t':
			switch state {
			case stateDocDash3, stateDocSpaces:
				separatorLength += 1
				state = stateDocSpaces
			case stateDocComment:
				separatorLength += 1
			default:
				state = stateRegularLine
			}
		case '#':
			switch state {
			case stateDocDash3, stateDocSpaces, stateDocComment:
				separatorLength += 1
				state = stateDocComment
			default:
				state = stateRegularLine
			}
		default:
			switch state {
			case stateDocComment:
				separatorLength += 1
			default:
				state = stateRegularLine
			}
		}
	}

	if docStartIndex != index+1 {
		switch state {
		case stateDocDash3, stateDocSpaces, stateDocComment:
			separatorLengthWithoutCursor := separatorLength - 1
			if docStartIndex == index-separatorLengthWithoutCursor {
				docContent = []byte{}
			} else {
				docContent = content[docStartIndex : index-separatorLengthWithoutCursor]
			}
		default:
			docContent = content[docStartIndex:]
		}
		docsContents = append(docsContents, docContent)
	}

	return docsContents
}

func emptyDocContent(content []byte) bool {
	const (
		stateRegular = 0
		stateComment = 1
	)

	state := stateRegular
	for _, ch := range content {
		switch ch {
		case '#':
			state = stateComment
		case '\n':
			state = stateRegular
		case ' ', '\r', '\t':
		default:
			if state == stateRegular {
				return false
			}
		}
	}
	return true
}

func prepareWerfConfig(rawImages []*rawStapelImage, rawImagesFromDockerfile []*rawImageFromDockerfile, meta *Meta) (*WerfConfig, error) {
	var stapelImages []*StapelImage
	var imagesFromDockerfile []*ImageFromDockerfile
	var artifacts []*StapelImageArtifact

	for _, rawImageFromDockerfile := range rawImagesFromDockerfile {
		if sameImages, err := rawImageFromDockerfile.toImageFromDockerfileDirectives(); err != nil {
			return nil, err
		} else {
			imagesFromDockerfile = append(imagesFromDockerfile, sameImages...)
		}
	}

	for _, rawImage := range rawImages {
		if rawImage.stapelImageType() == "images" {
			if sameImages, err := rawImage.toStapelImageDirectives(); err != nil {
				return nil, err
			} else {
				stapelImages = append(stapelImages, sameImages...)
			}
		} else {
			if imageArtifact, err := rawImage.toStapelImageArtifactDirectives(); err != nil {
				return nil, err
			} else {
				artifacts = append(artifacts, imageArtifact)
			}
		}
	}

	werfConfig := &WerfConfig{
		Meta:                 meta,
		StapelImages:         stapelImages,
		ImagesFromDockerfile: imagesFromDockerfile,
		Artifacts:            artifacts,
	}

	if err := werfConfig.validateImagesNames(); err != nil {
		return nil, err
	}

	if err := werfConfig.validateImagesFrom(); err != nil {
		return nil, err
	}

	if err := werfConfig.associateImportsArtifacts(); err != nil {
		return nil, err
	}

	if err := werfConfig.exportsAutoExcluding(); err != nil {
		return nil, err
	}

	if err := werfConfig.validateInfiniteLoopBetweenRelatedImages(); err != nil {
		return nil, err
	}

	return werfConfig, nil
}

func splitByMetaAndRawImages(docs []*doc) (*Meta, []*rawStapelImage, []*rawImageFromDockerfile, error) {
	var rawStapelImages []*rawStapelImage
	var rawImagesFromDockerfile []*rawImageFromDockerfile
	var resultMeta *Meta

	parentStack = util.NewStack()
	for _, doc := range docs {
		var raw map[string]interface{}
		err := yaml.UnmarshalStrict(doc.Content, &raw)
		if err != nil {
			return nil, nil, nil, newYamlUnmarshalError(err, doc)
		}

		if isMetaDoc(raw) {
			if resultMeta != nil {
				return nil, nil, nil, newYamlUnmarshalError(errors.New("duplicate meta config section definition"), doc)
			}

			rawMeta := &rawMeta{doc: doc}
			err := yaml.UnmarshalStrict(doc.Content, &rawMeta)
			if err != nil {
				return nil, nil, nil, newYamlUnmarshalError(err, doc)
			}

			resultMeta = rawMeta.toMeta()
		} else if isImageFromDockerfileDoc(raw) {
			imageFromDockerfile := &rawImageFromDockerfile{doc: doc}
			err := yaml.UnmarshalStrict(doc.Content, &imageFromDockerfile)
			if err != nil {
				return nil, nil, nil, newYamlUnmarshalError(err, doc)
			}

			rawImagesFromDockerfile = append(rawImagesFromDockerfile, imageFromDockerfile)
		} else if isImageDoc(raw) {
			image := &rawStapelImage{doc: doc}
			err := yaml.UnmarshalStrict(doc.Content, &image)
			if err != nil {
				return nil, nil, nil, newYamlUnmarshalError(err, doc)
			}

			rawStapelImages = append(rawStapelImages, image)
		} else {
			return nil, nil, nil, newYamlUnmarshalError(errors.New("cannot recognize type of config section (part of YAML stream separated by three hyphens, https://yaml.org/spec/1.2/spec.html#id2800132):\n * 'configVersion' required for meta config section;\n * 'image' required for the image config sections;\n * 'artifact' required for the artifact config sections;"), doc)
		}
	}

	return resultMeta, rawStapelImages, rawImagesFromDockerfile, nil
}

func isMetaDoc(h map[string]interface{}) bool {
	if _, ok := h["configVersion"]; ok {
		return true
	}

	return false
}

func isImageDoc(h map[string]interface{}) bool {
	if _, ok := h["image"]; ok {
		return true
	} else if _, ok := h["artifact"]; ok {
		return true
	}

	return false
}

func isImageFromDockerfileDoc(h map[string]interface{}) bool {
	if _, ok := h["dockerfile"]; ok {
		return true
	}

	return false
}

func newYamlUnmarshalError(err error, doc *doc) error {
	switch err.(type) {
	case *configError:
		return err
	default:
		message := err.Error()
		reg, err := regexp.Compile("line ([0-9]+)")
		if err != nil {
			return err
		}

		res := reg.FindStringSubmatch(message)

		if len(res) == 2 {
			line, err := strconv.Atoi(res[1])
			if err != nil {
				return err
			}

			message = reg.ReplaceAllString(message, fmt.Sprintf("line %d", line+doc.Line))
		}
		return newDetailedConfigError(message, nil, doc)
	}
}
