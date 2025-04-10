package gitCache

import (
	"time"

	"github.com/asjdf/lfs-s3/mod/gitCache/goblet"
	"github.com/asjdf/lfs-s3/mod/gitCache/source/github"
	"github.com/juanjiTech/jframe/core/kernel"
	"github.com/juanjiTech/jin"
	"github.com/pkg/errors"
)

var _ kernel.Module = (*Mod)(nil)

type GitHubConfig struct {
	AppId              string `yaml:"appId"`
	AppInstallationId  string `yaml:"appInstallationId"`
	AppPrivateKey      string `yaml:"appPrivateKey"`
	TokenExpirySeconds int    `yaml:"tokenExpirySeconds"`
}
type Config struct {
	CacheRoot string `yaml:"cacheRoot"` // root directory of cached repositories

	GitHub GitHubConfig `yaml:"github"`
}

type Mod struct {
	config Config
	kernel.UnimplementedModule
}

func (m *Mod) Name() string {
	return "gitCache"
}

func (m *Mod) Config() any {
	return &m.config
}

//func (m *Mod) PreInit(hub *kernel.Hub) error {
//
//	return nil
//}

//	func (m *Mod) Init(hub *kernel.Hub) error {
//		//TODO implement me
//		panic("implement me")
//	}
//
//	func (m *Mod) PostInit(hub *kernel.Hub) error {
//		//TODO implement me
//		panic("implement me")
//	}
func (m *Mod) Load(hub *kernel.Hub) error {
	var jinE *jin.Engine
	if hub.Load(&jinE) != nil {
		return errors.New("can't load jin.Engine from kernel")
	}

	ghConf := m.config.GitHub
	ts, err := github.NewTokenSource(
		ghConf.AppId,
		ghConf.AppInstallationId,
		ghConf.AppPrivateKey,
		time.Duration(ghConf.TokenExpirySeconds)*time.Second,
	)
	if err != nil {
		return err
	}
	authorizer := github.NewAuthorizer(true, nil)
	defer authorizer.Close()

	c := &goblet.ServerConfig{
		LocalDiskCacheRoot: m.config.CacheRoot,
		URLCanonializer:    github.URLCanonicalizer,
		RequestAuthorizer:  authorizer.RequestAuthorizer,
		TokenSource:        ts,
	}
	handler := goblet.HTTPHandler(c).ServeHTTP
	jinE.Any("/:org/:repo/info/refs", handler)
	jinE.Any("/:org/:repo/git-receive-pack", handler)
	jinE.Any("/:org/:repo/git-upload-pack", handler)
	return nil
}

func (m *Mod) Start(hub *kernel.Hub) error {
	return nil
}

//
//func (m *Mod) Stop(wg *sync.WaitGroup, ctx context.Context) error {
//	//TODO implement me
//	panic("implement me")
//}
