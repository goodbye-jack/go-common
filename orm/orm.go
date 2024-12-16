package orm

import (
	"context"
	"github.com/goodbye-jack/go-common/log"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type Orm struct {
	db *gorm.DB
}

func NewOrm(dsn string) *Orm {
	log.Info("NewOrm param:dsn=", dsn)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("mysql connect failed, %v", err)
	}
	return &Orm{
		db: db,
	}
}

func (o *Orm) AutoMigrate(ptr interface{}) {
	o.db.AutoMigrate(ptr)
}

func (o *Orm) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) {
	db := o.db.WithContext(ctx)
	db.Transaction(fn)
}

func (o *Orm) Create(ctx context.Context, ptr interface{}) error {
	db := o.db.WithContext(ctx)
	return db.Create(ptr).Error
}

func (o *Orm) First(ctx context.Context, res interface{}, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	return db.First(res, filters...).Error
}

func (o *Orm) Last(ctx context.Context, res interface{}, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	return db.Last(res, filters...).Error
}

func (o *Orm) FindAll(ctx context.Context, res interface{}, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Find(res).Error
	}
	return db.Find(res).Error
}

func (o *Orm) Page(ctx context.Context, res interface{}, page, pageSize int, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
	}
	return db.Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
}

func (o *Orm) Count(ctx context.Context, model interface{}, total int64, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Find(model).Count(&total).Error
	}
	return db.Find(model).Count(&total).Error
}

func (o *Orm) CountNew(ctx context.Context, res interface{}, total int64, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Find(res).Count(&total).Error
	}
	return db.Find(res).Count(&total).Error
}

func (o *Orm) Update(ctx context.Context, ptr interface{}) error {
	db := o.db.WithContext(ctx)
	return db.Save(ptr).Error
}

func (o *Orm) Delete(ctx context.Context, ptr interface{}) error {
	db := o.db.WithContext(ctx)
	return db.Delete(ptr).Error
}

func (o *Orm) Preload(key string, ctx context.Context, res interface{}, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		return db.Preload(key).Where(filters[0], filters[1:]...).Find(res).Error
	}
	return db.Preload(key).Find(res).Error
}

func (o *Orm) PagePerLoad(key string, ctx context.Context, res interface{}, page, pageSize int, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Preload(key).Limit(pageSize).Offset((page - 1) * pageSize).Find(&res).Error
	} else {
		return db.Preload(key).Limit(pageSize).Offset((page - 1) * pageSize).Find(&res).Error
	}
}
