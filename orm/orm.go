package orm

import (
	"context"
	"github.com/goodbye-jack/go-common/log"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"reflect"
)

type Orm struct {
	db *gorm.DB
}

func NewOrm(dsn string) *Orm {
	log.Info("NewOrm param:dsn=", dsn)
	// &gorm.Config{Logger: logger.Default.LogMode(logger.Info)}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Info)})
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

func (o *Orm) Preload(key string, ctx context.Context, res interface{}, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Preload(key).Find(res).Error
	}
	return db.Preload(key).Find(res).Error
}

func (o *Orm) Page(ctx context.Context, res interface{}, page, pageSize int, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
	}
	return db.Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
}

func (o *Orm) PagePerLoadCondition(key string, ctx context.Context, res interface{}, page, pageSize int, subKey string, subCondition string, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		//return db.Where(filters[0], filters[1:]...).Limit(pageSize).Offset((page-1)*pageSize).Preload(key, subKey+" = ?", subCondition).Find(res).Error
		if len(subKey) != 0 && len(subCondition) != 0 {
			return db.Where(filters[0], filters[1:]...).Limit(pageSize).Offset((page-1)*pageSize).Preload(key, subKey+" = ?", subCondition).Find(res).Error
		} else {
			return db.Where(filters[0], filters[1:]...).Limit(pageSize).Offset((page - 1) * pageSize).Preload(key).Find(res).Error
		}
	} else {
		//return db.Limit(pageSize).Offset((page-1)*pageSize).Preload(key, subKey+" = ?", subCondition).Find(res).Error
		if len(subKey) != 0 && len(subCondition) != 0 {
			return db.Limit(pageSize).Offset((page-1)*pageSize).Preload(key, subKey+" = ?", subCondition).Find(res).Error
		} else {
			return db.Limit(pageSize).Offset((page - 1) * pageSize).Preload(key).Find(res).Error
		}

	}
}

func (o *Orm) PagePerLoad(key string, ctx context.Context, res interface{}, page, pageSize int, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Limit(pageSize).Offset((page - 1) * pageSize).Preload(key).Find(res).Error
	} else {
		return db.Limit(pageSize).Offset((page - 1) * pageSize).Preload(key).Find(res).Error
	}
}

func (o *Orm) Count(ctx context.Context, table string, model interface{}, total int64, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	// 根据表名获取对应的模型结构体
	modelNew := reflect.New(reflect.TypeOf(model).Elem()).Interface()
	db.Table(table).Set("gorm:model", modelNew)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Count(&total).Error
	}
	return db.Count(&total).Error
}

func (o *Orm) Update(ctx context.Context, ptr interface{}) error {
	db := o.db.WithContext(ctx)
	return db.Save(ptr).Error
}

func (o *Orm) Delete(ctx context.Context, ptr interface{}) error {
	db := o.db.WithContext(ctx)
	return db.Delete(ptr).Error
}
