package changeguard

import (
	"context"
	"sync"
	"time"

	"github.com/goodbye-jack/go-common/log"
)

type workerManager struct {
	engine *Engine
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
}

func newWorkerManager(engine *Engine) *workerManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &workerManager{
		engine: engine,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start 按当前已注册的策略自动拉起后台 worker。
// 这里默认立即执行一轮，避免“服务刚启动但要等一个周期才处理”的空窗期。
func (m *workerManager) Start() {
	if m == nil || m.engine == nil {
		return
	}
	m.once.Do(func() {
		if m.engine.shouldRunNotificationWorker() {
			m.startLoop("notify", m.engine.opts.NotificationPollInterval, func(ctx context.Context) error {
				return m.engine.ProcessPendingNotifications(ctx, 50)
			})
		}
		if m.engine.shouldRunDriftWorker() {
			m.startLoop("drift", m.engine.opts.DriftPollInterval, func(ctx context.Context) error {
				return m.engine.RunDriftChecks(ctx)
			})
		}
	})
}

// startLoop 启动单个周期任务。
// 每个 worker 都独立 recover，避免某一类后台任务异常拖垮整个服务。
func (m *workerManager) startLoop(name string, interval time.Duration, fn func(context.Context) error) {
	if interval <= 0 || fn == nil {
		return
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		runWorkerSafely(name, func() error {
			return fn(m.ctx)
		})
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				runWorkerSafely(name, func() error {
					return fn(m.ctx)
				})
			}
		}
	}()
	log.Infof("changeguard worker started, service=%s, worker=%s, interval=%s", m.engine.opts.ServiceName, name, interval.String())
}

// Stop 用于后续接入统一生命周期管理时优雅关闭后台 worker。
func (m *workerManager) Stop() {
	if m == nil {
		return
	}
	m.cancel()
	m.wg.Wait()
}

// runWorkerSafely 保证后台任务失败只记录日志，不影响主业务请求链路。
func runWorkerSafely(name string, fn func() error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Warnf("changeguard worker panic recovered, worker=%s, panic=%v", name, recovered)
		}
	}()
	if err := fn(); err != nil {
		log.Warnf("changeguard worker run failed, worker=%s, err=%v", name, err)
	}
}
