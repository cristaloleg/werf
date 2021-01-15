package giterminism_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/gomega"

	"github.com/werf/werf/integration/pkg/utils"
)

func CommonBeforeEach() {
	gitInit()
	utils.CopyIn(utils.FixturePath("default"), SuiteData.TestDirPath)
	gitAddAndCommit("werf-giterminism.yaml")
	gitAddAndCommit("werf.yaml")
}

func gitInit() {
	utils.RunSucceedCommand(
		SuiteData.TestDirPath,
		"git",
		"init",
	)

	utils.RunSucceedCommand(
		SuiteData.TestDirPath,
		"git",
		"commit", "--allow-empty", "-m", "Initial commit",
	)
}

func gitAddAndCommit(relPath string) {
	utils.RunSucceedCommand(
		SuiteData.TestDirPath,
		"git",
		"add", relPath,
	)

	utils.RunSucceedCommand(
		SuiteData.TestDirPath,
		"git",
		"commit", "-m", fmt.Sprint("Update ", relPath),
	)
}

func fileCreateOrAppend(relPath, content string) {
	path := filepath.Join(SuiteData.TestDirPath, relPath)

	Ω(os.MkdirAll(filepath.Dir(path), 0777)).ShouldNot(HaveOccurred())

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	Ω(err).ShouldNot(HaveOccurred())

	_, err = f.WriteString(content)
	Ω(err).ShouldNot(HaveOccurred())

	Ω(f.Close()).ShouldNot(HaveOccurred())
}
