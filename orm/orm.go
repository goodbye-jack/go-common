package orm

import (
	"context"
	"fmt"
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

func (o *Orm) FindAllWithOrder(ctx context.Context, res interface{}, order interface{}, filters ...interface{}) error {
	db := o.db.WithContext(ctx).Order(order)
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

func (o *Orm) Page(ctx context.Context, res interface{}, page, pageSize int, sortColumn string, sortSc string, filters ...interface{}) error {
	sortBy := sortColumn + " " + sortSc
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Order(sortBy).Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
	}
	return db.Order(sortBy).Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
}

func (o *Orm) FindJoins(tableName string, ctx context.Context, res interface{}, returnRows, whereCondition string, joins ...string) error {
	db := o.db.WithContext(ctx).Table(tableName).Select(returnRows)
	for index, value := range joins {
		fmt.Printf("Index: %d, Value: %d\n", index, value)
		db.Joins(value)
	}
	return db.Where(whereCondition).Find(res).Error
}

func (o *Orm) PageJoins(tableName string, ctx context.Context, res interface{}, returnRows, whereCondition string, page, pageSize int, joins ...string) error {
	db := o.db.WithContext(ctx).Table(tableName).Select(returnRows)
	for index, value := range joins {
		fmt.Printf("Index: %d, Value: %d\n", index, value)
		db.Joins(value)
	}
	return db.Where(whereCondition).Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
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

func (o *Orm) PreloadCount(key string, ctx context.Context, res interface{}, total int64, filters ...interface{}) (int64, error) {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		db.Where(filters[0], filters[1:]...).Preload(key).Find(res).Count(&total)
		return total, nil
	}
	db.Preload(key).Find(res).Count(&total)
	return total, nil
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

func (o *Orm) GroupBy(ctx context.Context, tableName string, selectColumns string, whereClause interface{}, results interface{}, groupColumns string) error {
	db := o.db.WithContext(ctx)
	return db.Table(tableName).Select(selectColumns).Where(whereClause).Group(groupColumns).Find(results).Error
}

func (o *Orm) Top(ctx context.Context, tableName string, whereClause interface{}, groupColumn string, sortColumn string, sortSc string, limitCount int, results interface{}) error {
	db := o.db.WithContext(ctx)
	selectColumns := groupColumn + " , " + sortColumn
	sortBy := sortColumn + " " + sortSc
	return db.Table(tableName).Select(selectColumns).Where(whereClause).Group(groupColumn).Order(sortBy).Limit(limitCount).Scan(&results).Error
}
