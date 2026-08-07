package event

import "sync"

var (
	defaultEmitterMu sync.RWMutex
	defaultEmitter   *Emitter
)

func SetDefaultEmitter(emitter *Emitter) {
	defaultEmitterMu.Lock()
	defaultEmitter = emitter
	defaultEmitterMu.Unlock()
}

func DefaultEmitter() *Emitter {
	defaultEmitterMu.RLock()
	emitter := defaultEmitter
	defaultEmitterMu.RUnlock()
	return emitter
}
