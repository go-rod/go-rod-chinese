// This file serves for the Page.Evaluate.
// 这个文件是为 Page.Evaluate 服务

package rod

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/js"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
	"github.com/ysmood/gson"
)

// EvalOptions for Page.Evaluate
// Page.Evaluate 配置项
type EvalOptions struct {
	// If enabled the eval result will be a plain JSON value.
	// 如果启用，eval的结果会是普通JSON值
	// If disabled the eval result will be a reference of a remote js object.
	// 如果禁用，eval的结果会是一个JS对象的引用。
	ByValue bool

	AwaitPromise bool

	// ThisObj represents the "this" object in the JS
	// ThisObj 代表 JS 里面的 this
	ThisObj *proto.RuntimeRemoteObject

	// JS function definition to execute.
	// 要注入的 JS 函数
	JS string

	// JSArgs represents the arguments that will be passed to JS.
	// JSArgs 表示将被传递给 JS 函数的参数。
	// If an argument is *proto.RuntimeRemoteObject type, the corresponding remote object will be used.
	// 如果参数是 *proto.RuntimeRemoteObject 类型，将使用相应的远程对象。
	// Or it will be passed as a plain JSON value.
	// 或者它将作为普通JSON值传递。
	// When an arg in the args is a *js.Function, the arg will be cached on the page's js context.
	// 当args中的参数是*js.Function时，该参数将被缓存在页面的 js ctx中。
	// When the arg.Name exists in the page's cache, it reuse the cache without sending the definition to the browser again.
	// 当arg.Name存在于页面的缓存中时，它就会重新使用缓存，而不会再次将定义发送到浏览器。
	// Useful when you need to eval a huge js expression many times.
	// 当注入的 JS 体积非常大且要注入许多次时，是非常有效的。
	JSArgs []interface{}

	// Whether execution should be treated as initiated by user in the UI.
	// 在用户界面中是否应执行应由用户发起。
	UserGesture bool
}

// Eval creates a EvalOptions with ByValue set to true.
// 创建一个 ByValue 设置为 true 的 EvalOptions
func Eval(js string, args ...interface{}) *EvalOptions {
	return &EvalOptions{
		ByValue:      true,
		AwaitPromise: false,
		ThisObj:      nil,
		JS:           js,
		JSArgs:       args,
		UserGesture:  false,
	}
}

func evalHelper(fn *js.Function, args ...interface{}) *EvalOptions {
	return &EvalOptions{
		ByValue: true,
		JSArgs:  append([]interface{}{fn}, args...),
		JS:      `function (f, ...args) { return f.apply(this, args) }`,
	}
}

// String interface
func (e *EvalOptions) String() string {
	fn := e.JS
	args := e.JSArgs

	paramsStr := ""
	thisStr := ""

	if e.ThisObj != nil {
		thisStr = e.ThisObj.Description
	}
	if len(args) > 0 {
		if f, ok := args[0].(*js.Function); ok {
			fn = "rod." + f.Name
			args = e.JSArgs[1:]
		}

		paramsStr = strings.Trim(mustToJSONForDev(args), "[]\r\n")
	}

	return fmt.Sprintf("%s(%s) %s", fn, paramsStr, thisStr)
}

// This set the obj as ThisObj
// 设置 This 的值
func (e *EvalOptions) This(obj *proto.RuntimeRemoteObject) *EvalOptions {
	e.ThisObj = obj
	return e
}

// ByObject disables ByValue.
// 禁用 ByValue
func (e *EvalOptions) ByObject() *EvalOptions {
	e.ByValue = false
	return e
}

// ByUser enables UserGesture.
// 启用 UserGesture.
func (e *EvalOptions) ByUser() *EvalOptions {
	e.UserGesture = true
	return e
}

// ByPromise enables AwaitPromise.
// 启用 AwaitPromise
func (e *EvalOptions) ByPromise() *EvalOptions {
	e.AwaitPromise = true
	return e
}

func (e *EvalOptions) formatToJSFunc() string {
	js := strings.Trim(e.JS, "\t\n\v\f\r ;")
	return fmt.Sprintf(`function() { return (%s).apply(this, arguments) }`, js)
}

// Eval is a shortcut for Page.Evaluate with AwaitPromise, ByValue set to true.
// Eval 是 当 AwaitPromise ByValue 为 True 时的 Page.Evaluate 的快捷 API
func (p *Page) Eval(js string, args ...interface{}) (*proto.RuntimeRemoteObject, error) {
	return p.Evaluate(Eval(js, args...).ByPromise())
}

// Evaluate js on the page.
// 在页面中执行 JS
func (p *Page) Evaluate(opts *EvalOptions) (res *proto.RuntimeRemoteObject, err error) {
	var backoff utils.Sleeper

	// js context will be invalid if a frame is reloaded or not ready, then the isNilContextErr
	// will be true, then we retry the eval again.
	// 如果frame被重新加载或未准备好，js ctx 将无效，那么IsnilContexter将为true，然后会再次执行。
	for {
		res, err = p.evaluate(opts)
		if err != nil && errors.Is(err, cdp.ErrCtxNotFound) {
			if opts.ThisObj != nil {
				return nil, &ErrObjectNotFound{opts.ThisObj}
			}

			if backoff == nil {
				backoff = utils.BackoffSleeper(30*time.Millisecond, 3*time.Second, nil)
			} else {
				_ = backoff(p.ctx)
			}

			p.unsetJSCtxID()

			continue
		}
		return
	}
}

func (p *Page) evaluate(opts *EvalOptions) (*proto.RuntimeRemoteObject, error) {
	args, err := p.formatArgs(opts)
	if err != nil {
		return nil, err
	}

	req := proto.RuntimeCallFunctionOn{
		AwaitPromise:        opts.AwaitPromise,
		ReturnByValue:       opts.ByValue,
		UserGesture:         opts.UserGesture,
		FunctionDeclaration: opts.formatToJSFunc(),
		Arguments:           args,
	}

	if opts.ThisObj == nil {
		req.ObjectID, err = p.getJSCtxID()
		if err != nil {
			return nil, err
		}
	} else {
		req.ObjectID = opts.ThisObj.ObjectID
	}

	res, err := req.Call(p)
	if err != nil {
		return nil, err
	}

	if res.ExceptionDetails != nil {
		return nil, &ErrEval{res.ExceptionDetails}
	}

	return res.Result, nil
}

// Expose fn to the page's window object with the name. The exposure survives reloads.
// 将fn暴露给名为的页面窗口对象。exposure 在重新加载后仍然有效。
// Call stop to unbind the fn.
// 调用 stop 可以解除 fn 的绑定
func (p *Page) Expose(name string, fn func(gson.JSON) (interface{}, error)) (stop func() error, err error) {
	bind := "_" + utils.RandString(8)

	err = proto.RuntimeAddBinding{Name: bind}.Call(p)
	if err != nil {
		return
	}

	_, err = p.Evaluate(Eval(js.ExposeFunc.Definition, name, bind))
	if err != nil {
		return
	}

	code := fmt.Sprintf(`(%s)("%s", "%s")`, js.ExposeFunc.Definition, name, bind)
	remove, err := p.EvalOnNewDocument(code)
	if err != nil {
		return
	}

	p, cancel := p.WithCancel()

	stop = func() error {
		defer cancel()
		err := remove()
		if err != nil {
			return err
		}
		return proto.RuntimeRemoveBinding{Name: bind}.Call(p)
	}

	go p.EachEvent(func(e *proto.RuntimeBindingCalled) {
		if e.Name == bind {
			payload := gson.NewFrom(e.Payload)
			res, err := fn(payload.Get("req"))
			code := fmt.Sprintf("(res, err) => %s(res, err)", payload.Get("cb").Str())
			_, _ = p.Evaluate(Eval(code, res, err))
		}
	})()

	return
}

func (p *Page) formatArgs(opts *EvalOptions) ([]*proto.RuntimeCallArgument, error) {
	formated := []*proto.RuntimeCallArgument{}
	for _, arg := range opts.JSArgs {
		if obj, ok := arg.(*proto.RuntimeRemoteObject); ok { // remote object
			formated = append(formated, &proto.RuntimeCallArgument{ObjectID: obj.ObjectID})
		} else if obj, ok := arg.(*js.Function); ok { // js helper
			id, err := p.ensureJSHelper(obj)
			if err != nil {
				return nil, err
			}
			formated = append(formated, &proto.RuntimeCallArgument{ObjectID: id})
		} else { // plain json data
			formated = append(formated, &proto.RuntimeCallArgument{Value: gson.New(arg)})
		}
	}

	return formated, nil
}

// Check the doc of EvalHelper
// 检查 EvalHelper 的文档
func (p *Page) ensureJSHelper(fn *js.Function) (proto.RuntimeRemoteObjectID, error) {
	jsCtxID, err := p.getJSCtxID()
	if err != nil {
		return "", err
	}

	fnID, has := p.getHelper(jsCtxID, js.Functions.Name)
	if !has {
		res, err := proto.RuntimeCallFunctionOn{
			ObjectID:            jsCtxID,
			FunctionDeclaration: js.Functions.Definition,
		}.Call(p)
		if err != nil {
			return "", err
		}
		fnID = res.Result.ObjectID
		p.setHelper(jsCtxID, js.Functions.Name, fnID)
	}

	id, has := p.getHelper(jsCtxID, fn.Name)
	if !has {
		for _, dep := range fn.Dependencies {
			_, err := p.ensureJSHelper(dep)
			if err != nil {
				return "", err
			}
		}

		res, err := proto.RuntimeCallFunctionOn{
			ObjectID:  jsCtxID,
			Arguments: []*proto.RuntimeCallArgument{{ObjectID: fnID}},

			FunctionDeclaration: fmt.Sprintf(
				// we only need the object id, but the cdp will return the whole function string.
				// So we override the toString to reduce the overhead.
				// 通常只需要 对象ID就好，但是cdp会返回整个函数字符串，因此需要重写 toString 减少开销
				"functions => { const f = functions.%s = %s; f.toString = () => 'fn'; return f }",
				fn.Name, fn.Definition,
			),
		}.Call(p)
		if err != nil {
			return "", err
		}

		id = res.Result.ObjectID
		p.setHelper(jsCtxID, fn.Name, id)
	}

	return id, nil
}

func (p *Page) getHelper(jsCtxID proto.RuntimeRemoteObjectID, name string) (proto.RuntimeRemoteObjectID, bool) {
	p.helpersLock.Lock()
	defer p.helpersLock.Unlock()

	if p.helpers == nil {
		p.helpers = map[proto.RuntimeRemoteObjectID]map[string]proto.RuntimeRemoteObjectID{}
	}

	list, ok := p.helpers[jsCtxID]
	if !ok {
		list = map[string]proto.RuntimeRemoteObjectID{}
		p.helpers[jsCtxID] = list
	}

	id, ok := list[name]
	return id, ok
}

func (p *Page) setHelper(jsCtxID proto.RuntimeRemoteObjectID, name string, fnID proto.RuntimeRemoteObjectID) {
	p.helpersLock.Lock()
	defer p.helpersLock.Unlock()

	p.helpers[jsCtxID][name] = fnID
}

// Returns the page's window object, the page can be an iframe
// 返回页面窗口的对象，页面可以是一个 iframe
func (p *Page) getJSCtxID() (proto.RuntimeRemoteObjectID, error) {
	p.jsCtxLock.Lock()
	defer p.jsCtxLock.Unlock()

	if *p.jsCtxID != "" {
		return *p.jsCtxID, nil
	}

	if !p.IsIframe() {
		obj, err := proto.RuntimeEvaluate{Expression: "window"}.Call(p)
		if err != nil {
			return "", err
		}

		*p.jsCtxID = obj.Result.ObjectID
		p.helpersLock.Lock()
		p.helpers = nil
		p.helpersLock.Unlock()
		return *p.jsCtxID, nil
	}

	node, err := p.element.Describe(1, true)
	if err != nil {
		return "", err
	}

	obj, err := proto.DOMResolveNode{BackendNodeID: node.ContentDocument.BackendNodeID}.Call(p)
	if err != nil {
		return "", err
	}

	p.helpersLock.Lock()
	delete(p.helpers, *p.jsCtxID)
	p.helpersLock.Unlock()
	id, err := p.jsCtxIDByObjectID(obj.Object.ObjectID)
	*p.jsCtxID = id
	return *p.jsCtxID, err
}

func (p *Page) unsetJSCtxID() {
	p.jsCtxLock.Lock()
	defer p.jsCtxLock.Unlock()

	*p.jsCtxID = ""
}

func (p *Page) jsCtxIDByObjectID(id proto.RuntimeRemoteObjectID) (proto.RuntimeRemoteObjectID, error) {
	res, err := proto.RuntimeCallFunctionOn{
		ObjectID:            id,
		FunctionDeclaration: `() => window`,
	}.Call(p)
	if err != nil {
		return "", err
	}

	return res.Result.ObjectID, nil
}
