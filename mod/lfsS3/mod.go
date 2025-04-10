package lfsS3

import (
	"github.com/asjdf/lfs-s3/mod/lfsS3/handler"
	"github.com/asjdf/lfs-s3/mod/lfsS3/pkg/auth"
	"github.com/asjdf/lfs-s3/mod/lfsS3/storage"
	"github.com/juanjiTech/jframe/core/kernel"
	"github.com/juanjiTech/jin"
	"github.com/pkg/errors"
)

var _ kernel.Module = (*Mod)(nil)

type S3Config struct {
	Endpoint        string `yaml:"endpoint"`
	AccessKeyID     string `yaml:"accessKeyID"`
	SecretAccessKey string `yaml:"secretAccessKey"`
	BucketName      string `yaml:"bucketName"`
	Region          string `yaml:"region"`
}

type AuthConfig struct {
	EnableCache bool `yaml:"enableCache"`
}

type Config struct {
	S3   storage.S3Config `yaml:"s3"`
	Auth AuthConfig       `yaml:"auth"`
}

type Mod struct {
	config Config
	kernel.UnimplementedModule
}

func (m *Mod) Name() string {
	return "lfsS3"
}

func (m *Mod) Config() any {
	return &m.config
}

func (m *Mod) Load(hub *kernel.Hub) error {
	var jinE *jin.Engine
	if hub.Load(&jinE) != nil {
		return errors.New("can't load jin.Engine from kernel")
	}

	// 初始化S3存储
	s3Storage, err := storage.NewS3Storage(m.config.S3)
	if err != nil {
		return errors.Wrap(err, "failed to initialize S3 storage")
	}

	// 初始化鉴权器
	authorizer := auth.NewAuthorizer(m.config.Auth.EnableCache)
	defer authorizer.Close()

	// 创建并注册LFS处理器
	lfsHandler := handler.NewHandler(s3Storage, authorizer)
	lfsHandler.RegisterRoutes(jinE)

	return nil
}

func (m *Mod) Start(hub *kernel.Hub) error {
	return nil
}
