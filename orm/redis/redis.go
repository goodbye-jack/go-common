package redis

import (
	"context"
	"fmt"
	"github.com/goodbye-jack/go-common/orm/dbconfig"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/redis/go-redis/v9"
	"time"
)

// Redis Redis客户端封装（对齐ORM结构）
type Redis struct {
	client *redis.Client
	config *Config
	ctx    context.Context
}

// ========== 新增这行代码 ==========
// Client 暴露内部的redis.Client实例，供外部包提取配置
func (r *Redis) Client() *redis.Client {
	return r.client
}

// GetConfig 获取Redis配置（供外部包使用）
func (r *Redis) GetConfig() *Config {
	return r.config
}

//func (r *Redis) GetAddr() string {
//	if r.config == nil {
//		return ""
//	}
//	return fmt.Sprintf("%s:%d", r.config.Host, r.config.Port)
//}
//
//// GetPassword 获取Redis密码
//func (r *Redis) GetPassword() string {
//	if r.config == nil {
//		return ""
//	}
//	return r.config.Password
//}

//// GetDB 获取Redis DB索引
//func (r *Redis) GetDB() int {
//	if r.config == nil {
//		return 0
//	}
//	// 优先用config.DB，兜底用Database字段转int
//	if r.config.DB != 0 {
//		return r.config.DB
//	}
//	var db int
//	fmt.Sscanf(r.config.Database, "%d", &db)
//	return db
//}

// NewRedis 初始化Redis客户端（核心修复：复用通用GenDSN，删除重复逻辑）
func NewRedis(dsn string, dbType utils.DBType, timeout int, cfgFromYaml ...*dbconfig.Config) *Redis {
	if dbType != DBType {
		panic(fmt.Sprintf("unsupported db type: %s, expected: %s", dbType, DBType))
	}
	// ========== 核心1：初始化Config，复用通用dbconfig.Config ==========
	cfg := &Config{
		Config: dbconfig.Config{
			DSN:            dsn,                                  // 传入的DSN（优先级最低）
			DBType:         dbType,                               // 固定为redis
			ConnectTimeout: time.Duration(timeout) * time.Second, // 连接超时
		},
	}
	// ========== 核心2：映射yaml配置（覆盖默认值，优先级最高） ==========
	// 覆盖为yaml解析的原始配置（优先级最高）
	if len(cfgFromYaml) > 0 {
		cfg.Config = *cfgFromYaml[0] // 直接替换，持有Host/Port/Password等所有参数
	}
	// ========== 核心3：调用通用GenDSN生成Redis连接串（关键！） ==========
	// 把生成的DSN赋值回Config，方便后续查看
	cfg.DSN = cfg.GenDSN()

	// ========== 核心4：解析DSN并初始化客户端（容错逻辑） ==========
	opt, err := redis.ParseURL(cfg.GenDSN())
	if err != nil { // 解析失败时手动构建（兜底，兼容genRedisDSN的复杂格式）
		fmt.Printf("警告：解析DSN [%s] 失败 %v，手动构建连接配置\n", cfg.GenDSN(), err)
		var dbIndex int
		fmt.Sscanf(cfg.Database, "%d", &dbIndex)
		opt = &redis.Options{
			Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Password:     cfg.Password,
			DB:           dbIndex,
			DialTimeout:  cfg.ConnectTimeout,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		}
	}
	// 初始化客户端
	client := redis.NewClient(opt)
	// ========== 核心5：验证连接（带详细日志） ==========
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()
	fmt.Printf("尝试连接Redis | 地址：%s | DB：%d | 超时：%ds\n", opt.Addr, opt.DB, timeout)
	_, err = client.Ping(ctx).Result()
	if err != nil {
		panic(fmt.Sprintf("Redis Ping失败：%v | 最终DSN：%s | 连接地址：%s", err, cfg.GenDSN(), opt.Addr))
	}
	fmt.Printf("Redis初始化成功 | 地址：%s | DB：%d\n", opt.Addr, opt.DB)
	return &Redis{
		client: client,
		config: cfg,
		ctx:    context.Background(),
	}
}

//// NewRedis 修复版：正确解析配置 + 优先使用配置文件参数
//// dsn: 连接串（可为空，优先用配置文件的host/port）
//// dbType: 数据库类型
//// timeout: 初始化超时（秒）
//// 【新增】cfgFromYaml: 从yaml解析的完整配置（核心：传递配置文件参数）
//// 注意：需修改调用方，传入yaml解析后的dbconfig.Config
//func NewRedis(dsn string, dbType dbconfig.DBType, timeout int, cfgFromYaml ...dbconfig.Config) *Redis {
//	if dbType != DBType {
//		panic(fmt.Sprintf("unsupported db type: %s, expected: %s", dbType, DBType))
//	}
//
//	// ========== 核心修复1：初始化Config并映射yaml配置 ==========
//	cfg := &Config{
//		Config: dbconfig.Config{
//			DSN:            dsn,
//			DBType:         dbType,
//			ConnectTimeout: time.Duration(timeout) * time.Second,
//			Database:       "0", // 默认DB 0
//		},
//	}
//
//	// 映射yaml配置（优先级：yaml配置 > 传入的dsn）
//	if len(cfgFromYaml) > 0 {
//		yamlCfg := cfgFromYaml[0]
//		cfg.Host = yamlCfg.Host
//		cfg.Port = yamlCfg.Port
//		cfg.Password = yamlCfg.Password
//		cfg.Database = yamlCfg.Database
//		cfg.Mode = yamlCfg.Mode
//		// 覆盖DSN：优先用yaml配置生成，而非传入的空dsn
//		cfg.DSN = "" // 清空原有DSN，强制重新生成
//	}
//
//	// ========== 核心修复2：正确调用GenDSN并更新cfg.DSN ==========
//	// 之前的问题：_ = cfg.GenDSN() 未赋值回cfg.DSN
//	cfg.GenDSN() // 现在GenDSN会自动更新cfg.DSN
//
//	// ========== 核心修复3：容错解析DSN ==========
//	opt, err := redis.ParseURL(cfg.DSN)
//	if err != nil {
//		// 解析失败时，手动构建opt（兜底方案）
//		fmt.Printf("警告：解析DSN失败 %v，使用host/port手动构建配置\n", err)
//		opt = &redis.Options{
//			Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
//			Password: cfg.Password,
//			DB:       cfg.GetDB(), // 复用你写的GetDB方法
//		}
//	}
//
//	// 初始化客户端
//	client := redis.NewClient(opt)
//
//	// ========== 核心修复4：增加超时日志 + 延长超时时间（可选） ==========
//	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
//	defer cancel()
//	fmt.Printf("尝试连接Redis：%s (超时时间：%d秒)\n", opt.Addr, timeout)
//	_, err = client.Ping(ctx).Result()
//	if err != nil {
//		// 详细报错信息，方便排查
//		panic(fmt.Sprintf("redis ping failed: %v | 连接地址：%s | 密码：%s | DB：%d",
//			err, opt.Addr, opt.Password, opt.DB))
//	}
//
//	fmt.Printf("Redis连接成功：%s\n", opt.Addr)
//	return &Redis{
//		client: client,
//		config: cfg,
//		ctx:    context.Background(),
//	}
//}

//// NewRedis 初始化Redis客户端（对齐orm.NewOrm） dsn: 连接串 | dbType: 固定为DBTypeRedis | timeout: 初始化超时（秒）
//func NewRedis(dsn string, dbType dbconfig.DBType, timeout int) *Redis {
//	if dbType != DBType {
//		panic(fmt.Sprintf("unsupported db type: %s, expected: %s", dbType, DBType))
//	}
//	// 解析配置
//	cfg := &Config{
//		Config: dbconfig.Config{
//			DSN:            dsn,
//			DBType:         dbType,
//			ConnectTimeout: time.Duration(timeout) * time.Second,
//			// Redis的DB索引存在Database字段中
//			Database: "0", // 默认DB 0
//		},
//	}
//	_ = cfg.GenDSN() // 自动生成DSN（若为空）
//
//	// 解析Redis URL（官方go-redis/v9的ParseURL方法）
//	opt, err := redis.ParseURL(cfg.DSN)
//	if err != nil {
//		panic(fmt.Sprintf("redis parse dsn failed: %v", err))
//	}
//	// 初始化客户端（官方API）
//	client := redis.NewClient(opt)
//	// 验证连接（官方Result()返回2个值：(string, error)）
//	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
//	defer cancel()
//	_, err = client.Ping(ctx).Result() // 修复赋值数量不匹配问题
//	if err != nil {
//		panic(fmt.Sprintf("redis ping failed: %v", err))
//	}
//	return &Redis{
//		client: client,
//		config: cfg,
//		ctx:    context.Background(), // 基础上下文
//	}
//}

// 以下保留你的原有方法，API与官方go-redis/v9完全兼容，无需修改
// Close 关闭连接
func (r *Redis) Close() error {
	return r.client.Close()
}

// Set 设置KV（带过期时间）
func (r *Redis) Set(ctx context.Context, key string, value interface{}, expire time.Duration) error {
	if ctx == nil {
		ctx = r.ctx
	}
	return r.client.Set(ctx, key, value, expire).Err()
}

// Get 获取KV
func (r *Redis) Get(ctx context.Context, key string) (string, error) {
	if ctx == nil {
		ctx = r.ctx
	}
	return r.client.Get(ctx, key).Result()
}

// Del 删除KV
func (r *Redis) Del(ctx context.Context, keys ...string) (int64, error) {
	if ctx == nil {
		ctx = r.ctx
	}
	return r.client.Del(ctx, keys...).Result()
}

// HSet 哈希设置
func (r *Redis) HSet(ctx context.Context, key string, values ...interface{}) (int64, error) {
	if ctx == nil {
		ctx = r.ctx
	}
	return r.client.HSet(ctx, key, values...).Result()
}

// HGet 哈希获取
func (r *Redis) HGet(ctx context.Context, key, field string) (string, error) {
	if ctx == nil {
		ctx = r.ctx
	}
	return r.client.HGet(ctx, key, field).Result()
}

// HGetAll 哈希获取所有
func (r *Redis) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	if ctx == nil {
		ctx = r.ctx
	}
	return r.client.HGetAll(ctx, key).Result()
}

// LPush 列表左推
func (r *Redis) LPush(ctx context.Context, key string, values ...interface{}) (int64, error) {
	if ctx == nil {
		ctx = r.ctx
	}
	return r.client.LPush(ctx, key, values...).Result()
}

// RPop 列表右弹
func (r *Redis) RPop(ctx context.Context, key string) (string, error) {
	if ctx == nil {
		ctx = r.ctx
	}
	return r.client.RPop(ctx, key).Result()
}

// SAdd 集合添加
func (r *Redis) SAdd(ctx context.Context, key string, members ...interface{}) (int64, error) {
	if ctx == nil {
		ctx = r.ctx
	}
	return r.client.SAdd(ctx, key, members...).Result()
}

// SMembers 集合获取所有成员
func (r *Redis) SMembers(ctx context.Context, key string) ([]string, error) {
	if ctx == nil {
		ctx = r.ctx
	}
	return r.client.SMembers(ctx, key).Result()
}

// Expire 设置过期时间
func (r *Redis) Expire(ctx context.Context, key string, expire time.Duration) (bool, error) {
	if ctx == nil {
		ctx = r.ctx
	}
	return r.client.Expire(ctx, key, expire).Result()
}

// Transaction 事务操作（适配官方TxPipelined API）
// Transaction 事务操作（修复类型断言错误，适配go-redis/v9正确用法）
//
//	func exampleUseTransaction(redisClient *Redis) error {
//		ctx := context.Background()
//		// 执行Redis事务
//		return redisClient.Transaction(ctx, func(pipe redis.Pipeliner) error {
//			// 事务内执行指令（所有指令通过pipe调用，自动纳入事务）
//			pipe.Set(ctx, "key1", "value1", 0)
//			pipe.HSet(ctx, "hash1", "field1", "val1")
//			pipe.SAdd(ctx, "set1", "member1")
//
//			// 若返回error，事务会自动回滚
//			// return errors.New("手动触发回滚")
//			return nil
//		})
//	}
func (r *Redis) Transaction(ctx context.Context, fn func(pipe redis.Pipeliner) error) error {
	if ctx == nil {
		ctx = r.ctx
	}
	// TxPipelined 本身就是事务管道，直接传递Pipeliner给回调函数即可
	_, err := r.client.TxPipelined(ctx, fn)
	return err
}

// Ping 验证连接
func (r *Redis) Ping(ctx context.Context) error {
	if ctx == nil {
		ctx = r.ctx
	}
	_, err := r.client.Ping(ctx).Result()
	return err
}

// GetClient 获取原始客户端（兼容底层API）
func (r *Redis) GetClient() *redis.Client {
	return r.client
}
