package modList

import (
	"github.com/asjdf/lfs-s3/mod/jinx"
	"github.com/asjdf/lfs-s3/mod/lfsS3"
	"github.com/juanjiTech/jframe/core/kernel"
)

var ModList = []kernel.Module{
	&jinx.Mod{},
	&lfsS3.Mod{},
}
