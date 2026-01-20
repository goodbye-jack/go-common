package mongodb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/orm/dbconfig"
	"github.com/goodbye-jack/go-common/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Mongo Mongo客户端封装（对齐ORM结构）
type Mongo struct {
	client     *mongo.Client
	database   *mongo.Database
	config     *Config
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewMongo 初始化Mongo客户端（修复所有逻辑错误）
// 参数说明：
//
//	dsn: Mongo连接字符串（如mongodb://127.0.0.1:27017/test_db?ssl=false）
//	dbType: 数据库类型（必须是mongo/mongodb）
//	timeout: 连接超时时间（秒）
func NewMongo(dsn string, dbType utils.DBType, timeout int) *Mongo {
	// 1. 校验数据库类型
	if dbType != utils.DBTypeMongo && dbType != utils.DBTypeMongoDB {
		panic(fmt.Sprintf("unsupported db type: %s, expected: mongo/mongodb", dbType))
	}
	// 2. 初始化配置结构体
	cfg := &Config{
		DSN:            dsn,
		DBType:         dbType,
		ConnectTimeout: time.Duration(timeout) * time.Second,
	}
	// 3. 关键步骤：从DSN中解析出数据库名（核心修复）
	dbName, err := parseDBNameFromDSN(dsn)
	if err != nil {
		panic(fmt.Sprintf("parse database name from dsn failed: %v", err))
	}
	// 将解析出的数据库名赋值给Config的Database字段
	cfg.Database = dbName
	// 4. 校验数据库名非空（此时才是有效的校验）
	if cfg.Database == "" {
		panic("mongo config error: database name cannot be empty (check dsn: mongodb://host:port/[dbname]?xxx)")
	}
	// 5. 构建Mongo客户端连接选项
	clientOpts := options.Client().ApplyURI(dsn)
	// 填充连接池参数（从配置/默认值）
	if cfg.MaxPoolSize <= 0 {
		cfg.MaxPoolSize = utils.DefaultMongoMaxPoolSize // 使用默认值20
	}
	if cfg.MinPoolSize <= 0 {
		cfg.MinPoolSize = utils.DefaultMongoMinPoolSize // 使用默认值5
	}
	clientOpts.SetMaxPoolSize(uint64(cfg.MaxPoolSize))
	clientOpts.SetMinPoolSize(uint64(cfg.MinPoolSize))
	// 6. 建立连接（带超时上下文）
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		cancel()
		panic(fmt.Sprintf("mongo connect failed: %v", err))
	}
	// 7. 验证连接有效性
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		cancel()
		panic(fmt.Sprintf("mongo ping failed: %v", err))
	}
	// 8. 核心赋值：初始化数据库实例（给Mongo结构体的database字段）
	db := client.Database(cfg.Database)
	if db == nil {
		cancel()
		panic(fmt.Sprintf("failed to get database instance: %s", cfg.Database))
	}
	// 9. 返回初始化完成的Mongo客户端
	return &Mongo{
		client:     client,
		database:   db, // 关键：赋值后insertOne才能正常使用
		config:     cfg,
		ctx:        ctx,
		cancelFunc: cancel,
	}
}

// parseDBNameFromDSN 从Mongo的DSN中解析数据库名
// DSN格式：mongodb://host:port/[dbname]?param1=value1&param2=value2
func parseDBNameFromDSN(dsn string) (string, error) {
	// 解析URL
	uri, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("invalid dsn format: %v", err)
	}

	// 提取路径（数据库名）：/test_db → test_db
	dbName := strings.TrimPrefix(uri.Path, "/")
	// 处理空路径（无数据库名）
	if dbName == "" || dbName == "/" {
		return "", fmt.Errorf("dsn missing database name (format: mongodb://host:port/[dbname]?xxx)")
	}

	return dbName, nil
}

//// NewMongo 初始化Mongo客户端（对齐orm.NewOrm）
//// dsn: 连接串 | dbType: 固定为DBTypeMongo | timeout: 初始化超时（秒）
//func NewMongo(dsn string, dbType dbconfig.DBType, timeout int) *Mongo {
//	if dbType != DBType {
//		panic(fmt.Sprintf("unsupported db type: %s, expected: %s", dbType, DBType))
//	}
//	// 解析配置（优先用DSN，无则自动生成）
//	cfg := &Config{
//		DSN:            dsn,
//		DBType:         dbType,
//		ConnectTimeout: time.Duration(timeout) * time.Second,
//	}
//	_ = cfg.GenDSN() // 若DSN为空，自动生成
//	// 初始化上下文
//	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
//	// 建立连接
//	clientOpts := options.Client().ApplyURI(cfg.DSN)
//	// 设置连接池（复用配置）
//	clientOpts.SetMaxPoolSize(uint64(cfg.MaxPoolSize))
//	clientOpts.SetMinPoolSize(uint64(cfg.MinPoolSize))
//
//	client, err := mongo.Connect(ctx, clientOpts)
//	if err != nil {
//		cancel()
//		panic(fmt.Sprintf("mongo connect failed: %v", err))
//	}
//	// 验证连接
//	if err := client.Ping(ctx, readpref.Primary()); err != nil {
//		cancel()
//		panic(fmt.Sprintf("mongo ping failed: %v", err))
//	}
//	// 初始化数据库实例
//	db := client.Database(cfg.Database)
//	return &Mongo{
//		client:     client,
//		database:   db,
//		config:     cfg,
//		ctx:        ctx,
//		cancelFunc: cancel,
//	}
//}

// GenMongoURI 从通用Config生成Mongo连接串
func GenMongoURI(config *dbconfig.Config) string { // 优先级：Config中已配置DSN > 拼接URI
	if config.DSN != "" {
		return config.DSN
	}
	var uri string // 基础拼接逻辑（支持无密码/有密码场景）
	if config.User != "" && config.Password != "" {
		uri = fmt.Sprintf("mongodb://%s:%s@%s:%d/", config.User, config.Password, config.Host, config.Port)
	} else {
		uri = fmt.Sprintf("mongodb://%s:%d/", config.Host, config.Port)
	}
	params := []string{} // 附加数据库名和SSL等参数
	if config.AuthDB != "" {
		params = append(params, fmt.Sprintf("authSource=%s", config.AuthDB))
	}
	if config.SSLMode != "" {
		params = append(params, fmt.Sprintf("ssl=%s", config.SSLMode))
	}
	if len(params) > 0 {
		uri += fmt.Sprintf("%s?%s", config.Database, strings.Join(params, "&"))
	} else {
		uri += config.Database
	}
	return uri
}

// Close 关闭连接（优雅退出）
func (m *Mongo) Close() error {
	m.cancelFunc()
	return m.client.Disconnect(m.ctx)
}

// Collection 获取集合（类似ORM的Table）
func (m *Mongo) Collection(name string) *mongo.Collection {
	return m.database.Collection(name)
}

// CmdExec 1. 基础命令执行
func CmdExec(command string) (error, string, string) {
	cmd := exec.Command("bash", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return err, stdout.String(), stderr.String()
}

// 2. 客户端/数据库基础操作
// GetClient 获取原始客户端（兼容底层API，整合原GetClient）
func (m *Mongo) GetClient() (*mongo.Client, error) {
	if m.client == nil {
		return nil, errors.New("client 是空")
	}
	return m.client, nil
}

// ListDatabaseNames 列出数据库名称
func (m *Mongo) ListDatabaseNames(ctx context.Context, filter bson.D) ([]string, error) {
	if m.client == nil {
		return nil, errors.New("client 是空")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	return m.client.ListDatabaseNames(ctx, filter)
}

// DbDel 删除指定数据库
func (m *Mongo) DbDel(ctx context.Context) error {
	if m.client == nil {
		return fmt.Errorf("未查询到相关数据库配置信息")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := m.client.Database(m.config.Database).Drop(dbCtx)
	if err != nil {
		log.Errorf("db delete error:%s", err.Error())
		return fmt.Errorf("数据库delete error: %v", err)
	}
	return nil
}

// CollectionExport 3. 数据导入导出  导出集合为JSON文件
func (m *Mongo) CollectionExport(collName string, path string) (error, string, string) {
	if m.config == nil {
		return fmt.Errorf("未查询到相关数据库配置信息"), "", ""
	}
	dumpCmd := fmt.Sprintf("mongoexport --forceTableScan -h %s:%s -d %s -c %s -o %s",
		m.config.Host, m.config.Port, m.config.Database, collName, path+"/"+collName+".json")
	return CmdExec(dumpCmd)
}

// CollectionRestore 恢复集合数据
func (m *Mongo) CollectionRestore(path string) (error, string, string) {
	if m.config == nil {
		return fmt.Errorf("未查询到相关数据库配置信息"), "", ""
	}
	restoreCmd := fmt.Sprintf("mongorestore -h %s:%s -d %s -o %s",
		m.config.Host, m.config.Port, m.config.Database, path)
	return CmdExec(restoreCmd)
}

// DbDump 整库备份（mongodump）
func (m *Mongo) DbDump(path string) (error, string, string) {
	if m.config == nil { // 校验Config
		return fmt.Errorf("未查询到相关数据库配置信息"), "", ""
	}
	if m.config.Database == "" {
		return fmt.Errorf("Config中Database字段不能为空"), "", ""
	}
	if _, err := os.Stat(path); os.IsNotExist(err) { // 检查并创建备份目录
		if err := os.MkdirAll(path, 0755); err != nil {
			log.Error("DbDump 创建目录失败", errors.New(err.Error())) // 替换为你项目的日志
			return err, "", ""
		}
	}
	mongoURI := GenMongoURI(m.config) // 生成mongodump命令（从Config获取URI/数据库名）
	restoreCmd := fmt.Sprintf("mongodump --uri='%s' -d '%s' --out='%s'", mongoURI, m.config.Database, path)
	log.Info("DbDump 执行命令", fmt.Sprintf("cmd", restoreCmd))
	err, str1, str2 := CmdExec(restoreCmd) // 执行命令（保持原有cmdExec逻辑）
	log.Info("DbDump 执行结果", fmt.Sprintf("stdout", str1), fmt.Sprintf("stderr", str2))
	if err != nil {
		log.Error("DbDump 执行失败", errors.New(err.Error()))
	}
	return err, str1, str2
}

// DbRestore 整库恢复（mongorestore）
func (m *Mongo) DbRestore(path string) (error, string, string) {
	if m.config == nil {
		return fmt.Errorf("未查询到相关数据库配置信息"), "", ""
	}
	//restoreCmd := fmt.Sprintf("mongorestore --drop --uri='%s' -d '%s' '%s'", m.config.Uri, m.config.Database, path)
	restoreCmd := fmt.Sprintf("mongorestore --drop --uri='%s' -d '%s' '%s'", GenMongoURI(m.config), m.config.Database, path)
	log.Infof("DbRestore:%s", restoreCmd)
	err, str1, str2 := CmdExec(restoreCmd)
	log.Infof("str1:%s str2:%s", str1, str2)
	if err != nil {
		log.Errorf("err:%s", err.Error())
	}
	return err, str1, str2
}

// 4. 聚合查询
// aggregate 聚合查询底层方法
func (m *Mongo) aggregate(ctx context.Context, collectionName string, pipeline []bson.M, timeout int) ([]bson.M, error) {
	if m.client == nil {
		return nil, fmt.Errorf("未获取到client")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	if timeout <= 0 {
		timeout = 10
	}
	dbCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	collection := m.database.Collection(collectionName)
	opts := &options.AggregateOptions{}
	cursor, err := collection.Aggregate(dbCtx, pipeline, opts)
	if nil != err {
		return nil, fmt.Errorf("查询失败,%v", err)
	}
	defer func() {
		if nil == cursor {
			return
		}
		if err = cursor.Close(dbCtx); err != nil {
			log.Error(fmt.Sprintf("聚合查询失败,%v", err))
		}
	}()
	var results []bson.M
	for cursor.Next(dbCtx) {
		var result bson.M
		err = cursor.Decode(&result)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

// Aggregate 聚合查询（默认超时10s）
func (m *Mongo) Aggregate(collectionName string, pipeline []bson.M) ([]bson.M, error) {
	return m.aggregate(context.Background(), collectionName, pipeline, 10)
}

// AggregateWithTimeout 自定义超时的聚合查询
func (m *Mongo) AggregateWithTimeout(collectionName string, pipeline []bson.M, timeout int) ([]bson.M, error) {
	return m.aggregate(context.Background(), collectionName, pipeline, timeout)
}

// AggregateWithCtx 带上下文的聚合查询
func (m *Mongo) AggregateWithCtx(ctx context.Context, collectionName string, pipeline []bson.M) ([]bson.M, error) {
	return m.aggregate(ctx, collectionName, pipeline, 10)
}

// 5. 分页查询
// findPage 分页查询底层方法
func (m *Mongo) findPage(ctx context.Context, collectionName string, filter bson.M, pageIndex, pageSize int64, sort bson.D) ([]bson.M, int64, error) {
	if m.client == nil {
		return nil, 0, fmt.Errorf("未获取到client")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	opts := &options.FindOptions{}
	opts.SetLimit(pageSize)
	opts.SetSkip((pageIndex - 1) * pageSize)
	opts.SetSort(sort)
	collection := m.database.Collection(collectionName)
	cursor, err := collection.Find(dbCtx, filter, opts)
	if nil != err {
		return nil, 0, fmt.Errorf("查询失败,%v", err)
	}
	defer func() {
		if nil == cursor {
			return
		}
		if err = cursor.Close(dbCtx); err != nil {
			log.Error(fmt.Sprintf("单表按条件查询失败,%v", err))
		}
	}()
	var results []bson.M
	for cursor.Next(dbCtx) {
		var result bson.M
		err = cursor.Decode(&result)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, result)
	}
	total, _ := collection.CountDocuments(dbCtx, filter)
	return results, total, nil
}

// FindPage 分页查询（默认上下文）
func (m *Mongo) FindPage(filter bson.M, collectionName string, pageIndex, pageSize int64, sort bson.D) ([]bson.M, int64, error) {
	return m.findPage(context.Background(), collectionName, filter, pageIndex, pageSize, sort)
}

// FindPageWithCtx 带上下文的分页查询
func (m *Mongo) FindPageWithCtx(ctx context.Context, collectionName string, filter bson.M, pageIndex, pageSize int64, sort bson.D) ([]bson.M, int64, error) {
	return m.findPage(ctx, collectionName, filter, pageIndex, pageSize, sort)
}

// 6. 计数查询
// getCount 计数查询底层方法
func (m *Mongo) getCount(ctx context.Context, collectionName string, filter bson.M) int64 {
	if m.client == nil {
		return 0
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collectionName)
	total, err := collection.CountDocuments(dbCtx, filter)
	if err != nil {
		log.Errorf("CountDocuments error %s", err.Error())
	}
	return total
}

// GetCount 计数查询（默认上下文）
func (m *Mongo) GetCount(collectionName string, filter bson.M) int64 {
	return m.getCount(context.Background(), collectionName, filter)
}

// GetCountWithCtx 带上下文的计数查询
func (m *Mongo) GetCountWithCtx(ctx context.Context, collectionName string, filter bson.M) int64 {
	return m.getCount(ctx, collectionName, filter)
}

// 7. 批量查询
// findMany 批量查询底层方法（默认限制500条）
func (m *Mongo) findMany(ctx context.Context, collectionName string, filter bson.M) ([]bson.M, error) {
	if m.client == nil {
		return nil, fmt.Errorf("未获取到client")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collectionName)
	opts := &options.FindOptions{}
	opts.SetLimit(500)
	cursor, err := collection.Find(dbCtx, filter, opts)
	if nil != err {
		return nil, fmt.Errorf("查询失败,%v", err)
	}
	defer func() {
		if nil == cursor {
			return
		}
		if err = cursor.Close(dbCtx); err != nil {
			log.Error(fmt.Sprintf("单表按条件查询失败,%v", err))
		}
	}()
	var results []bson.M
	for cursor.Next(dbCtx) {
		var result bson.M
		err = cursor.Decode(&result)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

// FindMany 批量查询（默认上下文）
func (m *Mongo) FindMany(collectionName string, filter bson.M) ([]bson.M, error) {
	return m.findMany(context.Background(), collectionName, filter)
}

// FindManyWithCtx 带上下文的批量查询
func (m *Mongo) FindManyWithCtx(ctx context.Context, collectionName string, filter bson.M) ([]bson.M, error) {
	return m.findMany(ctx, collectionName, filter)
}

// findManyWithOptions 带自定义选项的批量查询底层方法
func (m *Mongo) findManyWithOptions(collectionName string, ctx context.Context, filter bson.M, opts *options.FindOptions) ([]bson.M, error) {
	if m.client == nil {
		return nil, fmt.Errorf("未获取到client")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collectionName)
	cursor, err := collection.Find(dbCtx, filter, opts)
	if nil != err {
		return nil, fmt.Errorf("查询失败,%v", err)
	}
	defer func() {
		if nil == cursor {
			return
		}
		if err = cursor.Close(dbCtx); err != nil {
			log.Error(fmt.Sprintf("单表按条件查询失败,%v", err))
		}
	}()
	var results []bson.M
	for cursor.Next(dbCtx) {
		var result bson.M
		err = cursor.Decode(&result)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

// FindManyWithOptions 带自定义选项的批量查询（默认上下文）
func (m *Mongo) FindManyWithOptions(collectionName string, filter bson.M, opts *options.FindOptions) ([]bson.M, error) {
	return m.findManyWithOptions(collectionName, context.Background(), filter, opts)
}

// FindManyWithOptionsAndCtx 带上下文+自定义选项的批量查询
func (m *Mongo) FindManyWithOptionsAndCtx(collectionName string, ctx context.Context, filter bson.M, opts *options.FindOptions) ([]bson.M, error) {
	return m.findManyWithOptions(collectionName, ctx, filter, opts)
}

// 8. 单条查询
// findOne 单条查询底层方法（记录耗时）
func (m *Mongo) findOne(collectionName string, ctx context.Context, filter bson.M) (bson.M, error) {
	if m.client == nil {
		return nil, fmt.Errorf("未获取到client")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	st1 := time.Now()
	collection := m.database.Collection(collectionName)
	et1 := time.Since(st1)
	if et1 > 100*time.Millisecond {
		log.Warnf("filter= %s , 获取collection耗时较大 %s ", filter, et1)
	} else {
		log.Infof("filter= %s , 获取collection耗时 %s ", filter, et1)
	}
	var result bson.M
	st2 := time.Now()
	err := collection.FindOne(dbCtx, filter).Decode(&result)
	et2 := time.Since(st2)
	if et2 > 100*time.Millisecond {
		log.Warnf("filter= %s, FindOne解码耗时较大 %s ", filter, et2)
	} else {
		log.Infof("filter= %s , FindOne解码耗时 %s ", filter, et2)
	}
	if err != nil && err.Error() != "mongo: no documents in result" {
		return nil, err
	}
	return result, nil
}

// FindOne 单条查询（默认上下文）
func (m *Mongo) FindOne(collectionName string, filter bson.M) (bson.M, error) {
	return m.findOne(collectionName, context.Background(), filter)
}

// FindOneWithCtx 带上下文的单条查询
func (m *Mongo) FindOneWithCtx(collName string, ctx context.Context, filter bson.M) (bson.M, error) {
	return m.findOne(collName, ctx, filter)
}

// 9. 插入操作
// insertOne 插入单条数据底层方法
func (m *Mongo) insertOne(collName string, ctx context.Context, model interface{}) (interface{}, error) {
	if m.client == nil {
		return nil, fmt.Errorf("未获取到client")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collName)
	insertResult, err := collection.InsertOne(dbCtx, model)
	if nil == insertResult {
		return 0, err
	}
	return insertResult.InsertedID, err
}

// InsertOneWithCtx 带上下文插入单条数据
func (m *Mongo) InsertOneWithCtx(ctx context.Context, collName string, model interface{}) (interface{}, error) {
	return m.insertOne(collName, ctx, model)
}

// insertMany 插入多条数据底层方法
func (m *Mongo) insertMany(collName string, ctx context.Context, model []interface{}) ([]interface{}, error) {
	if m.client == nil {
		return nil, fmt.Errorf("未获取到client")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collName)
	insertResult, err := collection.InsertMany(dbCtx, model)
	if nil == insertResult {
		return nil, err
	}
	return insertResult.InsertedIDs, err
}

// InsertMany 插入多条数据（默认上下文）
func (m *Mongo) InsertMany(collName string, model []interface{}) ([]interface{}, error) {
	return m.insertMany(collName, context.Background(), model)
}

// InsertManyWithCtx 带上下文插入多条数据
func (m *Mongo) InsertManyWithCtx(collName string, ctx context.Context, model []interface{}) ([]interface{}, error) {
	return m.insertMany(collName, ctx, model)
}

// 10. 更新操作
// updateOne 更新单条数据底层方法
func (m *Mongo) updateOne(collName string, ctx context.Context, filter interface{}, model interface{}) (int64, error) {
	if m.client == nil {
		return 0, fmt.Errorf("未获取到client")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collName)
	updateResult, err := collection.UpdateOne(dbCtx, filter, bson.M{"$set": model})
	if nil == updateResult {
		return 0, err
	}
	return updateResult.ModifiedCount, err
}

// // UpdateOne 更新单条数据
//
//	func (m *Mongo) UpdateOne(ctx context.Context, collName string, filter interface{}, update interface{}) (*mongo.UpdateResult, error) {
//		if ctx == nil {
//			ctx = m.ctx
//		}
//		return m.Collection(collName).UpdateOne(ctx, filter, update)
//	}
//
// UpdateOne 更新单条数据（默认上下文）
func (m *Mongo) UpdateOne(collName string, filter interface{}, model interface{}) (int64, error) {
	return m.updateOne(collName, context.Background(), filter, model)
}

// UpdateOneWithCtx 带上下文更新单条数据
func (m *Mongo) UpdateOneWithCtx(collName string, ctx context.Context, filter interface{}, model interface{}) (int64, error) {
	return m.updateOne(collName, ctx, filter, model)
}

// updateOneWithOptions 带自定义选项更新单条数据底层方法
func (m *Mongo) updateOneWithOptions(collName string, ctx context.Context, filter interface{}, model interface{}, opts *options.UpdateOptions) (int64, error) {
	if m.client == nil {
		return 0, fmt.Errorf("未获取到client")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collName)
	updateResult, err := collection.UpdateOne(dbCtx, filter, bson.M{"$set": model}, opts)
	if nil == updateResult {
		return 0, err
	}
	return updateResult.ModifiedCount, err
}

// UpdateOneWithOptions 带自定义选项更新单条数据（默认上下文）
func (m *Mongo) UpdateOneWithOptions(collName string, filter interface{}, model interface{}, opts *options.UpdateOptions) (int64, error) {
	return m.updateOneWithOptions(collName, context.Background(), filter, model, opts)
}

// UpdateOneWithOptionsAndCtx 带上下文+自定义选项更新单条数据
func (m *Mongo) UpdateOneWithOptionsAndCtx(collName string, ctx context.Context, filter interface{}, model interface{}, opts *options.UpdateOptions) (int64, error) {
	return m.updateOneWithOptions(collName, ctx, filter, model, opts)
}

// updateMany 更新多条数据底层方法
func (m *Mongo) updateMany(collName string, ctx context.Context, filter interface{}, model interface{}) (int64, error) {
	if m.client == nil {
		return 0, fmt.Errorf("未获取到client")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collName)
	updateResult, err := collection.UpdateMany(dbCtx, filter, bson.M{"$set": model})
	if updateResult == nil {
		return 0, err
	}
	return updateResult.ModifiedCount, err
}

// UpdateMany 更新多条数据（默认上下文）
func (m *Mongo) UpdateMany(collName string, filter interface{}, model interface{}) (int64, error) {
	return m.updateMany(collName, context.Background(), filter, model)
}

// UpdateManyWithCtx 带上下文更新多条数据
func (m *Mongo) UpdateManyWithCtx(collName string, ctx context.Context, filter interface{}, model interface{}) (int64, error) {
	return m.updateMany(collName, ctx, filter, model)
}

// updateManyWithOptions 带自定义选项更新多条数据底层方法
func (m *Mongo) updateManyWithOptions(collName string, ctx context.Context, filter, model interface{}, opts *options.UpdateOptions) (int64, error) {
	if m.client == nil {
		return 0, fmt.Errorf("未获取到client")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collName)
	updateRes, err := collection.UpdateMany(dbCtx, filter, bson.M{"$set": model}, opts)
	if nil == updateRes {
		return 0, err
	}
	return updateRes.ModifiedCount, err
}

// UpdateManyWithOptions 带自定义选项更新多条数据（默认上下文）
func (m *Mongo) UpdateManyWithOptions(collName string, filter interface{}, model interface{}, opts *options.UpdateOptions) (int64, error) {
	return m.updateManyWithOptions(collName, context.Background(), filter, model, opts)
}

// 11. 删除操作
// deleteOne 删除单条数据底层方法
func (m *Mongo) deleteOne(collName string, ctx context.Context, filter bson.M) (int64, error) {
	if m.client == nil {
		return 0, fmt.Errorf("未获取到client")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collName)
	count, err := collection.DeleteOne(dbCtx, filter, nil)
	if nil == count {
		return 0, err
	}
	return count.DeletedCount, err
}

// // DeleteOne 删除单条数据
//
//	func (m *Mongo) DeleteOne(ctx context.Context, collName string, filter interface{}) (*mongo.DeleteResult, error) {
//		if ctx == nil {
//			ctx = m.ctx
//		}
//		return m.Collection(collName).DeleteOne(ctx, filter)
//	}
//
// DeleteOne 删除单条数据（默认上下文）
func (m *Mongo) DeleteOne(collName string, filter bson.M) (int64, error) {
	return m.deleteOne(collName, context.Background(), filter)
}

// DeleteOneWithCtx 带上下文删除单条数据
func (m *Mongo) DeleteOneWithCtx(collName string, ctx context.Context, filter bson.M) (int64, error) {
	return m.deleteOne(collName, ctx, filter)
}

// deleteMany 删除多条数据底层方法
func (m *Mongo) deleteMany(collName string, ctx context.Context, filter bson.M) (int64, error) {
	if m.client == nil {
		return 0, fmt.Errorf("未获取到client")
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collName)
	count, err := collection.DeleteMany(dbCtx, filter)
	if count == nil {
		return 0, err
	}
	return count.DeletedCount, err
}

// DeleteMany 删除多条数据（默认上下文）
func (m *Mongo) DeleteMany(collName string, filter bson.M) (int64, error) {
	return m.deleteMany(collName, context.Background(), filter)
}

// DeleteManyWithCtx 带上下文删除多条数据
func (m *Mongo) DeleteManyWithCtx(collName string, ctx context.Context, filter bson.M) (int64, error) {
	return m.deleteMany(collName, ctx, filter)
}

// CreateIndexWithModel 索引管理 通过IndexModel创建索引
func (m *Mongo) CreateIndexWithModel(collName string, model mongo.IndexModel) error {
	if m.client == nil {
		return errors.New(fmt.Sprintf("未获取到(%s)对应的mongo客户端", m.config.DBName))
	}
	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collName)
	_, err := collection.Indexes().CreateOne(dbCtx, model,
		options.CreateIndexes().SetMaxTime(10*time.Second),
	)
	return err
}

// CreateIndex 创建单字段索引（支持唯一索引）
func (m *Mongo) CreateIndex(collName string, column string, setUnique bool) error {
	return m.createIndex(collName, context.Background(), column, setUnique)
}

// CreateIndexWithCtx 带上下文创建单字段索引
func (m *Mongo) CreateIndexWithCtx(collName string, ctx context.Context, column string, setUnique bool) error {
	return m.createIndex(collName, ctx, column, setUnique)
}

// createIndex 创建索引底层方法
func (m *Mongo) createIndex(collName string, ctx context.Context, column string, setUnique bool) error {
	if m.client == nil {
		return errors.New(fmt.Sprintf("未获取到(%s)对应的mongo客户端", m.config.DBName))
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collName)
	_, err := collection.Indexes().CreateOne(
		dbCtx,
		mongo.IndexModel{
			Keys:    bson.M{column: 1},
			Options: options.Index().SetUnique(setUnique),
		}, options.CreateIndexes().SetMaxTime(10*time.Second),
	)
	return err
}

// CreateManyIndex 批量创建索引
func (m *Mongo) CreateManyIndex(collName string, indexs []bson.M) error {
	return m.createManyIndex(collName, context.Background(), indexs)
}

// createManyIndex 批量创建索引底层方法
func (m *Mongo) createManyIndex(collName string, ctx context.Context, indexs []bson.M) error {
	if len(indexs) <= 0 {
		return fmt.Errorf("无待创建索引")
	}
	if m.client == nil {
		return errors.New(fmt.Sprintf("未获取到(%s)对应的mongo客户端", m.config.DBName))
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collName)

	indexModels := make([]mongo.IndexModel, 0)
	for _, b := range indexs {
		indexModels = append(indexModels, mongo.IndexModel{
			Keys:    bson.M{b["column"].(string): b["sort"].(int)},
			Options: options.Index().SetUnique(b["unique"].(bool)),
		})
	}
	_, err := collection.Indexes().CreateMany(dbCtx, indexModels,
		options.CreateIndexes().SetMaxTime(10*time.Second))
	return err
}

// DropIndex 删除指定名称的索引
func (m *Mongo) DropIndex(collName string, name string) error {
	return m.dropIndex(collName, context.Background(), name)
}

// dropIndex 删除索引底层方法
func (m *Mongo) dropIndex(collName string, ctx context.Context, name string) error {
	if m.client == nil {
		return errors.New(fmt.Sprintf("未获取到(%s)对应的mongo客户端", m.config.DBName))
	}
	if ctx == nil {
		ctx = m.ctx
	}
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	collection := m.database.Collection(collName)
	_, err := collection.Indexes().DropOne(dbCtx, name)
	return err
}

// 13. 工具方法
// Bson2Struct BSON转结构体
func Bson2Struct(val interface{}, model interface{}) error {
	data, err := bson.Marshal(val)
	if err != nil {
		return err
	}
	err = bson.Unmarshal(data, model)
	return err
}

// ---------------------- 核心操作方法（对齐ORM调用风格） ----------------------

// Find 查询多条数据
func (m *Mongo) Find(ctx context.Context, collName string, filter interface{}, result interface{}, opts ...*options.FindOptions) error {
	if ctx == nil {
		ctx = m.ctx
	}
	cursor, err := m.Collection(collName).Find(ctx, filter, opts...)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	return cursor.All(ctx, result)
}

// Transaction 事务操作（Mongo 4.0+支持）
func (m *Mongo) Transaction(ctx context.Context, fn func(sessCtx mongo.SessionContext) error) error {
	if ctx == nil {
		ctx = m.ctx
	}
	session, err := m.client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		if err := fn(sessCtx); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return err
}

// Count 统计数量
func (m *Mongo) Count(ctx context.Context, collName string, filter interface{}) (int64, error) {
	if ctx == nil {
		ctx = m.ctx
	}
	return m.Collection(collName).CountDocuments(ctx, filter)
}

// Ping 验证连接可用性
func (m *Mongo) Ping(ctx context.Context) error {
	if ctx == nil {
		ctx = m.ctx
	}
	return m.client.Ping(ctx, readpref.Primary())
}

// GetDatabase 获取原始数据库实例
func (m *Mongo) GetDatabase() *mongo.Database {
	return m.database
}
