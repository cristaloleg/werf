package storage

import (
	"github.com/flant/werf/pkg/container_runtime"
	"github.com/flant/werf/pkg/image"
)

type ImagesRepo interface {
	DeleteRepo() error
	DeleteImageRepo(imageName string) error

	GetRepoImage(imageName, tag string) (*image.Info, error)
	GetRepoImages(imageNames []string) (map[string][]*image.Info, error)
	DeleteRepoImage(_ DeleteRepoImageOptions, repoImageList ...*image.Info) error

	GetAllImageRepoTags(imageName string) ([]string, error)
	PublishImage(publishImage *container_runtime.WerfImage) error

	CreateImageRepo(imageName string) error
	RemoveImageRepo(imageName string) error

	ImageRepositoryName(imageName string) string
	ImageRepositoryNameWithTag(imageName, tag string) string
	ImageRepositoryTag(imageName, tag string) string

	Validate() error
	String() string
}

type ImagesRepoOptions struct {
	DockerImagesRepoOptions
}

func NewImagesRepo(projectName, imagesRepoAddress, imagesRepoMode string, options ImagesRepoOptions) (ImagesRepo, error) {
	return NewDockerImagesRepo(projectName, imagesRepoAddress, imagesRepoMode, options.DockerImagesRepoOptions)
}
