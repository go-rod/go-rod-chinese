package rod

import (
	"context"
	"time"

	"github.com/go-rod/rod/lib/utils"
)

type timeoutContextKey struct{}
type timeoutContextVal struct {
	parent context.Context
	cancel context.CancelFunc
}

// Context 返回具有指定ctx的克隆，用于链式子操作
func (b *Browser) Context(ctx context.Context) *Browser {
	newObj := *b
	newObj.ctx = ctx
	return &newObj
}

// GetContext 获取当前的ctx实例
func (b *Browser) GetContext() context.Context {
	return b.ctx
}

// Timeout 返回一个克隆，其中包含所有链接子操作的指定总超时
func (b *Browser) Timeout(d time.Duration) *Browser {
	ctx, cancel := context.WithTimeout(b.ctx, d)
	return b.Context(context.WithValue(ctx, timeoutContextKey{}, &timeoutContextVal{b.ctx, cancel}))
}

// CancelTimeout 取消当前超时上下文，并返回具有父上下文的克隆
func (b *Browser) CancelTimeout() *Browser {
	val := b.ctx.Value(timeoutContextKey{}).(*timeoutContextVal)
	val.cancel()
	return b.Context(val.parent)
}

// WithCancel 返回带有上下文取消函数的克隆
func (b *Browser) WithCancel() (*Browser, func()) {
	ctx, cancel := context.WithCancel(b.ctx)
	return b.Context(ctx), cancel
}

// Sleeper 为链式子操作返回具有指定Sleeper的克隆
func (b *Browser) Sleeper(sleeper func() utils.Sleeper) *Browser {
	newObj := *b
	newObj.sleeper = sleeper
	return &newObj
}

// Context 返回具有指定ctx的克隆，用于链式子操作
func (p *Page) Context(ctx context.Context) *Page {
	newObj := *p
	newObj.ctx = ctx
	return &newObj
}

// GetContext 获取当前ctx实例
func (p *Page) GetContext() context.Context {
	return p.ctx
}

// Timeout 返回一个克隆，其中包含所有链接子操作的指定总超时
func (p *Page) Timeout(d time.Duration) *Page {
	ctx, cancel := context.WithTimeout(p.ctx, d)
	return p.Context(context.WithValue(ctx, timeoutContextKey{}, &timeoutContextVal{p.ctx, cancel}))
}

// CancelTimeout 取消当前超时上下文，并返回具有父上下文的克隆
func (p *Page) CancelTimeout() *Page {
	val := p.ctx.Value(timeoutContextKey{}).(*timeoutContextVal)
	val.cancel()
	return p.Context(val.parent)
}

// WithCancel 返回带有上下文取消函数的克隆
func (p *Page) WithCancel() (*Page, func()) {
	ctx, cancel := context.WithCancel(p.ctx)
	return p.Context(ctx), cancel
}

// Sleeper 为链式子操作返回具有指定Sleeper的克隆
func (p *Page) Sleeper(sleeper func() utils.Sleeper) *Page {
	newObj := *p
	newObj.sleeper = sleeper
	return &newObj
}

// Context 返回具有指定ctx的克隆，用于链式子操作
func (el *Element) Context(ctx context.Context) *Element {
	newObj := *el
	newObj.ctx = ctx
	return &newObj
}

// GetContext 获取当前ctx的实例
func (el *Element) GetContext() context.Context {
	return el.ctx
}

// Timeout 返回一个克隆，其中包含所有链接子操作的指定总超时
func (el *Element) Timeout(d time.Duration) *Element {
	ctx, cancel := context.WithTimeout(el.ctx, d)
	return el.Context(context.WithValue(ctx, timeoutContextKey{}, &timeoutContextVal{el.ctx, cancel}))
}

// CancelTimeout 取消当前超时上下文，并返回具有父上下文的克隆
func (el *Element) CancelTimeout() *Element {
	val := el.ctx.Value(timeoutContextKey{}).(*timeoutContextVal)
	val.cancel()
	return el.Context(val.parent)
}

// WithCancel 返回带有上下文取消函数的克隆
func (el *Element) WithCancel() (*Element, func()) {
	ctx, cancel := context.WithCancel(el.ctx)
	return el.Context(ctx), cancel
}

// Sleeper 为链式子操作返回具有指定Sleeper的克隆
func (el *Element) Sleeper(sleeper func() utils.Sleeper) *Element {
	newObj := *el
	newObj.sleeper = sleeper
	return &newObj
}
