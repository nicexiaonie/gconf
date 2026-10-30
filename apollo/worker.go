package apollo

import "sync"

// worker 串行处理远端变更，避免并发落盘与业务回调乱序。
// 每个 namespace 只保留最新一条待处理变更：突发变更合并中间版本，但最新版本不会丢失，内存占用有上限。
type worker struct {
	syncer *Syncer

	mu      sync.Mutex
	pending map[string]Change
	closed  bool
	started bool

	wake chan struct{}
	quit chan struct{}
	done chan struct{}
	once sync.Once
}

// newWorker 创建 worker，变更先进入待处理表，start 后才开始消费。
func newWorker(s *Syncer) *worker {
	return &worker{
		syncer:  s,
		pending: make(map[string]Change),
		wake:    make(chan struct{}, 1),
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// OnChange 实现 ChangeSink，由 agollo 为每次推送单独创建的 goroutine 调用，stop 之后仍可能被调用。
func (w *worker) OnChange(ch Change) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	// 回调 goroutine 之间无序，旧通知不能覆盖已暂存的新通知；过期判断与 handleChange 一致。
	if old, ok := w.pending[ch.Namespace]; ok && ch.NotificationID > 0 && ch.NotificationID < old.NotificationID {
		w.mu.Unlock()
		return
	}
	w.pending[ch.Namespace] = ch
	w.mu.Unlock()

	select {
	case w.wake <- struct{}{}:
	default:
		// 已有未消费的唤醒信号，worker 取走待处理表时会一并处理本次变更。
	}
}

func (w *worker) start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.started {
		return
	}
	w.started = true
	go w.run()
}

func (w *worker) run() {
	defer close(w.done)
	for {
		select {
		case <-w.quit:
			return
		case <-w.wake:
		}
		w.mu.Lock()
		batch := w.pending
		w.pending = make(map[string]Change)
		w.mu.Unlock()
		for _, ch := range batch {
			select {
			case <-w.quit:
				return
			default:
			}
			w.syncer.handleChange(ch)
		}
	}
}

// stop 停止消费并等待正在处理的变更完成，幂等。
func (w *worker) stop() {
	w.once.Do(func() {
		w.mu.Lock()
		w.closed = true
		started := w.started
		w.mu.Unlock()
		close(w.quit)
		if started {
			<-w.done
		}
	})
}
