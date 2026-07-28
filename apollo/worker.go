package apollo

import "sync"

// worker 串行处理远端变更，避免并发落盘与业务回调乱序。
type worker struct {
	syncer *Syncer
	ch     chan Change
	done   chan struct{}
	once   sync.Once
}

func newWorker(s *Syncer) *worker {
	w := &worker{
		syncer: s,
		ch:     make(chan Change, 64),
		done:   make(chan struct{}),
	}
	go w.run()
	return w
}

// OnChange 实现 ChangeSink，由 agollo listener goroutine 调用。
func (w *worker) OnChange(ch Change) {
	select {
	case w.ch <- ch:
	default:
		// 队列满时丢弃，依赖下次变更或周期拉取兜底。
	}
}

func (w *worker) run() {
	for ch := range w.ch {
		w.syncer.handleChange(ch)
	}
	close(w.done)
}

func (w *worker) stop() {
	w.once.Do(func() {
		close(w.ch)
	})
	<-w.done
}
