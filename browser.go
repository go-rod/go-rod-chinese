//go:generate go run ./lib/utils/setup
//go:generate go run ./lib/proto/generate
//go:generate go run ./lib/js/generate
//go:generate go run ./lib/assets/generate
//go:generate go run ./lib/utils/lint

package rod

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/defaults"
	"github.com/go-rod/rod/lib/devices"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
	"github.com/ysmood/goob"
)

// Browser 实现了这些接口
var _ proto.Client = &Browser{}
var _ proto.Contextable = &Browser{}

// Browser 代表的是 browser.
// 它不依赖于文件系统，它可以与远程浏览器无缝工作。
// 要检查可用于从CLI快速启用选项的环境变量，请检查此处：
// https://pkg.go.dev/github.com/go-rod/rod/lib/defaults
type Browser struct {
	// BrowserContextID是隐身窗口的id。
	BrowserContextID proto.BrowserBrowserContextID

	e eFunc

	ctx context.Context

	sleeper func() utils.Sleeper

	logger utils.Logger

	slowMotion time.Duration // 查看 defaults.slow
	trace      bool          // 查看 defaults.Trace
	monitor    string

	defaultDevice devices.Device

	controlURL  string
	client      CDPClient
	event       *goob.Observable // 来自cdp客户端的所有浏览器事件
	targetsLock *sync.Mutex

	// 存储之前所有相同类型的cdp调用。浏览器没有足够的API让我们检索它所有的内部状态。这是一个变通办法，把它们映射到本地。
	// 例如，你不能使用cdp API来获取鼠标的当前位置。
	states *sync.Map
}

// 新创建一个浏览器控制器.
// 模拟设备的DefaultDevice被设置为devices.LaptopWithMDPIScreen.Landescape()，它可以使实际视图区域 小于浏览器窗口，你可以使用NoDefaultDevice来禁用它。
func New() *Browser {
	return (&Browser{
		ctx:           context.Background(),
		sleeper:       DefaultSleeper,
		controlURL:    defaults.URL,
		slowMotion:    defaults.Slow,
		trace:         defaults.Trace,
		monitor:       defaults.Monitor,
		logger:        DefaultLogger,
		defaultDevice: devices.LaptopWithMDPIScreen.Landescape(),
		targetsLock:   &sync.Mutex{},
		states:        &sync.Map{},
	}).WithPanic(utils.Panic)
}

// Incognito 创建了一个无痕浏览器
func (b *Browser) Incognito() (*Browser, error) {
	res, err := proto.TargetCreateBrowserContext{}.Call(b)
	if err != nil {
		return nil, err
	}

	incognito := *b
	incognito.BrowserContextID = res.BrowserContextID

	return &incognito, nil
}

// ControlURL设置远程控制浏览器的URL。
func (b *Browser) ControlURL(url string) *Browser {
	b.controlURL = url
	return b
}

// SlowMotion设置每个控制动作的延迟，如模拟人的输入。
func (b *Browser) SlowMotion(delay time.Duration) *Browser {
	b.slowMotion = delay
	return b
}

// Trace 启用/禁用 页面上输入动作的视觉追踪。
func (b *Browser) Trace(enable bool) *Browser {
	b.trace = enable
	return b
}

// 要侦听的监视器地址（如果不为空）。Browser.ServeMonitor的快捷方式
func (b *Browser) Monitor(url string) *Browser {
	b.monitor = url
	return b
}

// Logger覆盖了默认的日志功能，用于追踪
func (b *Browser) Logger(l utils.Logger) *Browser {
	b.logger = l
	return b
}

// Client 设置cdp的客户端
func (b *Browser) Client(c CDPClient) *Browser {
	b.client = c
	return b
}

// DefaultDevice为将来要模拟的新页面设置默认设备。
// 默认值是devices.LaptopWithMDPIScreen。
// 将其设置为devices.Clear来禁用它。
func (b *Browser) DefaultDevice(d devices.Device) *Browser {
	b.defaultDevice = d
	return b
}

// NoDefaultDevice is the same as DefaultDevice(devices.Clear)
func (b *Browser) NoDefaultDevice() *Browser {
	return b.DefaultDevice(devices.Clear)
}

// Connect 用于连接浏览器并且控制浏览器.
// 如果连接失败，尝试启动一个本地浏览器，如果没有找到本地浏览器，尝试下载一个。
func (b *Browser) Connect() error {
	if b.client == nil {
		u := b.controlURL
		if u == "" {
			var err error
			u, err = launcher.New().Context(b.ctx).Launch()
			if err != nil {
				return err
			}
		}

		c, err := cdp.StartWithURL(b.ctx, u, nil)
		if err != nil {
			return err
		}
		b.client = c
	}

	b.initEvents()

	if b.monitor != "" {
		launcher.Open(b.ServeMonitor(b.monitor))
	}

	return proto.TargetSetDiscoverTargets{Discover: true}.Call(b)
}

// Close 关闭浏览器
func (b *Browser) Close() error {
	if b.BrowserContextID == "" {
		return proto.BrowserClose{}.Call(b)
	}
	return proto.TargetDisposeBrowserContext{BrowserContextID: b.BrowserContextID}.Call(b)
}

// Page 创建一个新的浏览器标签。如果opts.URL为空，默认值将是 "about:blank"。
func (b *Browser) Page(opts proto.TargetCreateTarget) (p *Page, err error) {
	req := opts
	req.BrowserContextID = b.BrowserContextID
	req.URL = "about:blank"

	target, err := req.Call(b)
	if err != nil {
		return nil, err
	}
	defer func() {
		// 如果Navigate或PageFromTarget失败，我们应该关闭目标以防止泄漏
		if err != nil {
			_, _ = proto.TargetCloseTarget{TargetID: target.TargetID}.Call(b)
		}
	}()

	p, err = b.PageFromTarget(target.TargetID)
	if err != nil {
		return
	}

	if opts.URL == "" {
		return
	}

	err = p.Navigate(opts.URL)

	return
}

// Pages 检索所有可见页面
func (b *Browser) Pages() (Pages, error) {
	list, err := proto.TargetGetTargets{}.Call(b)
	if err != nil {
		return nil, err
	}

	pageList := Pages{}
	for _, target := range list.TargetInfos {
		if target.Type != proto.TargetTargetInfoTypePage {
			continue
		}

		page, err := b.PageFromTarget(target.TargetID)
		if err != nil {
			return nil, err
		}
		pageList = append(pageList, page)
	}

	return pageList, nil
}

// Call 用于直接调用原始cdp接口
func (b *Browser) Call(ctx context.Context, sessionID, methodName string, params interface{}) (res []byte, err error) {
	res, err = b.client.Call(ctx, sessionID, methodName, params)
	if err != nil {
		return nil, err
	}

	b.set(proto.TargetSessionID(sessionID), methodName, params)
	return
}

// PageFromSession 用于底层调试
func (b *Browser) PageFromSession(sessionID proto.TargetSessionID) *Page {
	sessionCtx, cancel := context.WithCancel(b.ctx)
	return &Page{
		e:             b.e,
		ctx:           sessionCtx,
		sessionCancel: cancel,
		sleeper:       b.sleeper,
		browser:       b,
		SessionID:     sessionID,
	}
}

// PageFromTarget 获取或创建一个Page实例。
func (b *Browser) PageFromTarget(targetID proto.TargetTargetID) (*Page, error) {
	b.targetsLock.Lock()
	defer b.targetsLock.Unlock()

	page := b.loadCachedPage(targetID)
	if page != nil {
		return page, nil
	}

	session, err := proto.TargetAttachToTarget{
		TargetID: targetID,
		Flatten:  true, // 如果没有设置，将不返回任何响应。
	}.Call(b)
	if err != nil {
		return nil, err
	}

	sessionCtx, cancel := context.WithCancel(b.ctx)

	page = &Page{
		e:             b.e,
		ctx:           sessionCtx,
		sessionCancel: cancel,
		sleeper:       b.sleeper,
		browser:       b,
		TargetID:      targetID,
		SessionID:     session.SessionID,
		FrameID:       proto.PageFrameID(targetID),
		jsCtxLock:     &sync.Mutex{},
		jsCtxID:       new(proto.RuntimeRemoteObjectID),
		helpersLock:   &sync.Mutex{},
	}

	page.root = page
	page.newKeyboard().newMouse().newTouch()

	if !b.defaultDevice.IsClear() {
		err = page.Emulate(b.defaultDevice)
		if err != nil {
			return nil, err
		}
	}

	b.cachePage(page)

	page.initEvents()

	// 如果我们不启用它，就会造成很多意想不到的浏览器行为。
	// 如proto.PageAddScriptToEvaluateOnNewDocument就不能工作。
	page.EnableDomain(&proto.PageEnable{})

	return page, nil
}

// EachEvent与Page.EachEvent类似，但可以捕获整个浏览器的事件。
func (b *Browser) EachEvent(callbacks ...interface{}) (wait func()) {
	return b.eachEvent("", callbacks...)
}

// WaitEvent 等待下一个事件的发生，时间为一次。它也会将数据加载到事件对象中。
func (b *Browser) WaitEvent(e proto.Event) (wait func()) {
	return b.waitEvent("", e)
}

// 等待下一个事件的发生，等待一次。它也会将数据加载到事件对象中。
func (b *Browser) waitEvent(sessionID proto.TargetSessionID, e proto.Event) (wait func()) {
	valE := reflect.ValueOf(e)
	valTrue := reflect.ValueOf(true)

	if valE.Kind() != reflect.Ptr {
		valE = reflect.New(valE.Type())
	}

	// 在运行时动态地创建一个函数:
	//
	// func(ee proto.Event) bool {
	//   *e = *ee
	//   return true
	// }
	fnType := reflect.FuncOf([]reflect.Type{valE.Type()}, []reflect.Type{valTrue.Type()}, false)
	fnVal := reflect.MakeFunc(fnType, func(args []reflect.Value) []reflect.Value {
		valE.Elem().Set(args[0].Elem())
		return []reflect.Value{valTrue}
	})

	return b.eachEvent(sessionID, fnVal.Interface())
}

// 如果任何回调返回true，事件循环将停止。
// 如果没有启用相关的domain，它将启用相domain，并在等待结束后恢复这些domain。
func (b *Browser) eachEvent(sessionID proto.TargetSessionID, callbacks ...interface{}) (wait func()) {
	cbMap := map[string]reflect.Value{}
	restores := []func(){}

	for _, cb := range callbacks {
		cbVal := reflect.ValueOf(cb)
		eType := cbVal.Type().In(0)
		name := reflect.New(eType.Elem()).Interface().(proto.Event).ProtoEvent()
		cbMap[name] = cbVal

		// 只有启用的domain才会向cdp客户端发出事件。
		// 如果没有启用相关domain，我们就为事件类型启用domain。
		// 在等待结束后，我们将domain恢复到它们之前的状态。
		domain, _ := proto.ParseMethodName(name)
		if req := proto.GetType(domain + ".enable"); req != nil {
			enable := reflect.New(req).Interface().(proto.Request)
			restores = append(restores, b.EnableDomain(sessionID, enable))
		}
	}

	b, cancel := b.WithCancel()
	messages := b.Event()

	return func() {
		if messages == nil {
			panic("can't use wait function twice")
		}

		defer func() {
			cancel()
			messages = nil
			for _, restore := range restores {
				restore()
			}
		}()

		for msg := range messages {
			if !(sessionID == "" || msg.SessionID == sessionID) {
				continue
			}

			if cbVal, has := cbMap[msg.Method]; has {
				e := reflect.New(proto.GetType(msg.Method))
				msg.Load(e.Interface().(proto.Event))
				args := []reflect.Value{e}
				if cbVal.Type().NumIn() == 2 {
					args = append(args, reflect.ValueOf(msg.SessionID))
				}
				res := cbVal.Call(args)
				if len(res) > 0 {
					if res[0].Bool() {
						return
					}
				}
			}
		}
	}
}

// Event 浏览器事件
func (b *Browser) Event() <-chan *Message {
	src := b.event.Subscribe(b.ctx)
	dst := make(chan *Message)
	go func() {
		defer close(dst)
		for {
			select {
			case <-b.ctx.Done():
				return
			case e, ok := <-src:
				if !ok {
					return
				}
				select {
				case <-b.ctx.Done():
					return
				case dst <- e.(*Message):
				}
			}
		}
	}()
	return dst
}

func (b *Browser) initEvents() {
	ctx, cancel := context.WithCancel(b.ctx)
	b.event = goob.New(ctx)
	event := b.client.Event()

	go func() {
		defer cancel()
		for e := range event {
			b.event.Publish(&Message{
				SessionID: proto.TargetSessionID(e.SessionID),
				Method:    e.Method,
				lock:      &sync.Mutex{},
				data:      e.Params,
			})
		}
	}()
}

func (b *Browser) pageInfo(id proto.TargetTargetID) (*proto.TargetTargetInfo, error) {
	res, err := proto.TargetGetTargetInfo{TargetID: id}.Call(b)
	if err != nil {
		return nil, err
	}
	return res.TargetInfo, nil
}

// IgnoreCertErrors 开关。如果启用，所有证书错误将被忽略。
func (b *Browser) IgnoreCertErrors(enable bool) error {
	return proto.SecuritySetIgnoreCertificateErrors{Ignore: enable}.Call(b)
}

// GetCookies 从浏览器获取Cookie
func (b *Browser) GetCookies() ([]*proto.NetworkCookie, error) {
	res, err := proto.StorageGetCookies{BrowserContextID: b.BrowserContextID}.Call(b)
	if err != nil {
		return nil, err
	}
	return res.Cookies, nil
}

// SetCookies 为浏览器设置Cookie，如果Cookie为nil则将所有Cookie清零
func (b *Browser) SetCookies(cookies []*proto.NetworkCookieParam) error {
	if cookies == nil {
		return proto.StorageClearCookies{BrowserContextID: b.BrowserContextID}.Call(b)
	}

	return proto.StorageSetCookies{
		Cookies:          cookies,
		BrowserContextID: b.BrowserContextID,
	}.Call(b)
}

// WaitDownload 返回一个helper，以获得下一个下载文件。
// 文件路径:
//     filepath.Join(dir, info.GUID)
func (b *Browser) WaitDownload(dir string) func() (info *proto.PageDownloadWillBegin) {
	var oldDownloadBehavior proto.BrowserSetDownloadBehavior
	has := b.LoadState("", &oldDownloadBehavior)

	_ = proto.BrowserSetDownloadBehavior{
		Behavior:         proto.BrowserSetDownloadBehaviorBehaviorAllowAndName,
		BrowserContextID: b.BrowserContextID,
		DownloadPath:     dir,
	}.Call(b)

	var start *proto.PageDownloadWillBegin

	waitProgress := b.EachEvent(func(e *proto.PageDownloadWillBegin) {
		start = e
	}, func(e *proto.PageDownloadProgress) bool {
		return start != nil && start.GUID == e.GUID && e.State == proto.PageDownloadProgressStateCompleted
	})

	return func() *proto.PageDownloadWillBegin {
		defer func() {
			if has {
				_ = oldDownloadBehavior.Call(b)
			} else {
				_ = proto.BrowserSetDownloadBehavior{
					Behavior:         proto.BrowserSetDownloadBehaviorBehaviorDefault,
					BrowserContextID: b.BrowserContextID,
				}.Call(b)
			}
		}()

		waitProgress()

		return start
	}
}

// Version 获取浏览器的版本信息
func (b *Browser) Version() (*proto.BrowserGetVersionResult, error) {
	return proto.BrowserGetVersion{}.Call(b)
}
