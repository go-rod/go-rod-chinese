package rod

import (
	"reflect"

	"github.com/go-rod/rod/lib/proto"
)

type stateKey struct {
	browserContextID proto.BrowserBrowserContextID
	sessionID        proto.TargetSessionID
	methodName       string
}

func (b *Browser) key(sessionID proto.TargetSessionID, methodName string) stateKey {
	return stateKey{
		browserContextID: b.BrowserContextID,
		sessionID:        sessionID,
		methodName:       methodName,
	}
}

func (b *Browser) set(sessionID proto.TargetSessionID, methodName string, params interface{}) {
	b.states.Store(b.key(sessionID, methodName), params)

	key := ""
	switch methodName {
	case (proto.EmulationClearDeviceMetricsOverride{}).ProtoReq():
		key = (proto.EmulationSetDeviceMetricsOverride{}).ProtoReq()
	case (proto.EmulationClearGeolocationOverride{}).ProtoReq():
		key = (proto.EmulationSetGeolocationOverride{}).ProtoReq()
	default:
		domain, name := proto.ParseMethodName(methodName)
		if name == "disable" {
			key = domain + ".enable"
		}
	}
	if key != "" {
		b.states.Delete(b.key(sessionID, key))
	}
}

// LoadState into the method, seesionID can be empty.
// 在方法的LoadState中，sessionID可以为空。
func (b *Browser) LoadState(sessionID proto.TargetSessionID, method proto.Request) (has bool) {
	data, has := b.states.Load(b.key(sessionID, method.ProtoReq()))
	if has {
		reflect.Indirect(reflect.ValueOf(method)).Set(
			reflect.Indirect(reflect.ValueOf(data)),
		)
	}
	return
}

// RemoveState a state
// 删除一个 state
func (b *Browser) RemoveState(key interface{}) {
	b.states.Delete(key)
}

// EnableDomain and returns a restore function to restore previous state
// EnableDomain 返回一个恢复函数来恢复之前的 State
func (b *Browser) EnableDomain(sessionID proto.TargetSessionID, req proto.Request) (restore func()) {
	_, enabled := b.states.Load(b.key(sessionID, req.ProtoReq()))

	if !enabled {
		_, _ = b.Call(b.ctx, string(sessionID), req.ProtoReq(), req)
	}

	return func() {
		if !enabled {
			domain, _ := proto.ParseMethodName(req.ProtoReq())
			_, _ = b.Call(b.ctx, string(sessionID), domain+".disable", nil)
		}
	}
}

// DisableDomain and returns a restore function to restore previous state
// DisableDomain 返回一个恢复函数来恢复之前的状态
func (b *Browser) DisableDomain(sessionID proto.TargetSessionID, req proto.Request) (restore func()) {
	_, enabled := b.states.Load(b.key(sessionID, req.ProtoReq()))
	domain, _ := proto.ParseMethodName(req.ProtoReq())

	if enabled {
		_, _ = b.Call(b.ctx, string(sessionID), domain+".disable", nil)
	}

	return func() {
		if enabled {
			_, _ = b.Call(b.ctx, string(sessionID), req.ProtoReq(), req)
		}
	}
}

func (b *Browser) cachePage(page *Page) {
	b.states.Store(page.TargetID, page)
}

func (b *Browser) loadCachedPage(id proto.TargetTargetID) *Page {
	if cache, ok := b.states.Load(id); ok {
		return cache.(*Page)
	}
	return nil
}

// LoadState into the method.
// 在方法中的LoadState
func (p *Page) LoadState(method proto.Request) (has bool) {
	return p.browser.LoadState(p.SessionID, method)
}

// EnableDomain and returns a restore function to restore previous state
// EnableDomain 返回一个恢复函数来恢复之前的 State
func (p *Page) EnableDomain(method proto.Request) (restore func()) {
	return p.browser.Context(p.ctx).EnableDomain(p.SessionID, method)
}

// DisableDomain and returns a restore function to restore previous state
// DisableDomain 返回一个恢复函数来恢复之前的状态。
func (p *Page) DisableDomain(method proto.Request) (restore func()) {
	return p.browser.Context(p.ctx).DisableDomain(p.SessionID, method)
}

func (p *Page) cleanupStates() {
	p.browser.RemoveState(p.TargetID)
}
