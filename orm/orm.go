package orm

import (
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/config"
)

type Orm struct {
	db *gorm.DB
}

var db *gorm.DB = nil

func init() {
	dsn := config.GetConfigString("tidb_dsn")
	if _db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{}); err != nil {
		log.Fatal("mysql connect failed, %v", err)
	} else {
		db = _db
	}
}

func NewOrm() *Orm {
	return &Orm {
		db: db,
	}
}

func (o *Orm) AutoMigrate(ptr interface{}) {
	o.db.AutoMigrate(ptr)
}

func (o *Orm) Transaction(fn func(tx *gorm.DB) error) {
	o.db.Transaction(fn)
}

func (o *Orm) Create(ptr interface{}) error {
	return o.db.Create(ptr).Error
}

func (o *Orm) First(res interface{}, filters ...interface{}) error {
	return o.db.First(res, filters...).Error
}

func (o *Orm) Last(res interface{}, filters ...interface{}) error {
	return o.db.Last(res, filters...).Error
}

func (o *Orm) FindAll(res interface{}, filters ...interface{}) error {
	if len(filters) > 0 {
		return o.db.Where(filters[0], filters[1:]...).Find(res).Error
	}
	return o.db.Find(res).Error
}

func (o *Orm) Page(res interface{}, page, pageSize int, filters ...interface{}) error {
	if len(filters) > 0 {
		return o.db.Where(filters[0], filters[1:]...).Limit(pageSize).Offset(page * pageSize).Find(res).Error
	}
	return o.db.Limit(pageSize).Offset(page * pageSize).Find(res).Error
}

func (o *Orm) Update(ptr interface{}) error {
	return o.db.Save(ptr).Error
}

func (o *Orm) Delete(ptr interface{}) error {
	return o.db.Delete(ptr).Error
}
