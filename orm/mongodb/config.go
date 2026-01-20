package mongodb

import (
	"github.com/goodbye-jack/go-common/orm/dbconfig"
	"github.com/goodbye-jack/go-common/utils"
)

// Config 复用dbconfig.Config，保持配置结构统一
type Config = dbconfig.Config

// DBType 快捷引用Mongo类型
const DBType = utils.DBTypeMongo
