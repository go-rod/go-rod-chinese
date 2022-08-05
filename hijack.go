package rod

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
	"github.com/ysmood/gson"
)

// HijackRequests 与Page.HijackRequests相同，但可以拦截整个浏览器的请求。
func (b *Browser) HijackRequests() *HijackRouter {
	return newHijackRouter(b, b).initEvents()
}

// HijackRequests 创建一个新的路由器实例，用于劫持请求。
// 当使用路由器以外的Fetch domain时，应该停止。启用劫持功能会禁用页面缓存。但诸如304 Not Modified等仍将按预期工作。
// 劫持一个请求的整个过程:
//    browser --req-> rod ---> server ---> rod --res-> browser
// The --req-> and --res-> 是可以修改的部分.
func (p *Page) HijackRequests() *HijackRouter {
	return newHijackRouter(p.browser, p).initEvents()
}

// HijackRouter 的context
type HijackRouter struct {
	run      func()
	stop     func()
	handlers []*hijackHandler
	enable   *proto.FetchEnable
	client   proto.Client
	browser  *Browser
}

func newHijackRouter(browser *Browser, client proto.Client) *HijackRouter {
	return &HijackRouter{
		enable:   &proto.FetchEnable{},
		browser:  browser,
		client:   client,
		handlers: []*hijackHandler{},
	}
}

func (r *HijackRouter) initEvents() *HijackRouter {
	ctx := r.browser.ctx
	if cta, ok := r.client.(proto.Contextable); ok {
		ctx = cta.GetContext()
	}

	var sessionID proto.TargetSessionID
	if tsa, ok := r.client.(proto.Sessionable); ok {
		sessionID = tsa.GetSessionID()
	}

	eventCtx, cancel := context.WithCancel(ctx)
	r.stop = cancel

	_ = r.enable.Call(r.client)

	r.run = r.browser.Context(eventCtx).eachEvent(sessionID, func(e *proto.FetchRequestPaused) bool {
		go func() {
			ctx := r.new(eventCtx, e)
			for _, h := range r.handlers {
				if !h.regexp.MatchString(e.Request.URL) {
					continue
				}

				h.handler(ctx)

				if ctx.continueRequest != nil {
					ctx.continueRequest.RequestID = e.RequestID
					err := ctx.continueRequest.Call(r.client)
					if err != nil {
						ctx.OnError(err)
					}
					return
				}

				if ctx.Skip {
					continue
				}

				if ctx.Response.fail.ErrorReason != "" {
					err := ctx.Response.fail.Call(r.client)
					if err != nil {
						ctx.OnError(err)
					}
					return
				}

				err := ctx.Response.payload.Call(r.client)
				if err != nil {
					ctx.OnError(err)
					return
				}
			}
		}()

		return false
	})
	return r
}

// 为路由添加一个 hijack handler,模式的文档与“proto.FetchRequestPattern.URLPattern”相同。
// 即使在调用“Run”之后，也可以添加新的handler.
func (r *HijackRouter) Add(pattern string, resourceType proto.NetworkResourceType, handler func(*Hijack)) error {
	r.enable.Patterns = append(r.enable.Patterns, &proto.FetchRequestPattern{
		URLPattern:   pattern,
		ResourceType: resourceType,
	})

	reg := regexp.MustCompile(proto.PatternToReg(pattern))

	r.handlers = append(r.handlers, &hijackHandler{
		pattern: pattern,
		regexp:  reg,
		handler: handler,
	})

	return r.enable.Call(r.client)
}

// Remove 通过 pattern 删除 handler
func (r *HijackRouter) Remove(pattern string) error {
	patterns := []*proto.FetchRequestPattern{}
	handlers := []*hijackHandler{}
	for _, h := range r.handlers {
		if h.pattern != pattern {
			patterns = append(patterns, &proto.FetchRequestPattern{URLPattern: h.pattern})
			handlers = append(handlers, h)
		}
	}
	r.enable.Patterns = patterns
	r.handlers = handlers

	return r.enable.Call(r.client)
}

// new 新建一个ctx
func (r *HijackRouter) new(ctx context.Context, e *proto.FetchRequestPaused) *Hijack {
	headers := http.Header{}
	for k, v := range e.Request.Headers {
		headers[k] = []string{v.String()}
	}

	u, _ := url.Parse(e.Request.URL)

	req := &http.Request{
		Method: e.Request.Method,
		URL:    u,
		Body:   ioutil.NopCloser(strings.NewReader(e.Request.PostData)),
		Header: headers,
	}

	return &Hijack{
		Request: &HijackRequest{
			event: e,
			req:   req.WithContext(ctx),
		},
		Response: &HijackResponse{
			payload: &proto.FetchFulfillRequest{
				ResponseCode: 200,
				RequestID:    e.RequestID,
			},
			fail: &proto.FetchFailRequest{
				RequestID: e.RequestID,
			},
		},
		OnError: func(err error) {},

		browser: r.browser,
	}
}

// Run the router, after you call it, you shouldn't add new handler to it.
// Run 运行 router 在你调用它之后，就再不能给它添加新的 handler。
func (r *HijackRouter) Run() {
	r.run()
}

// Stop 停止router
func (r *HijackRouter) Stop() error {
	r.stop()
	return proto.FetchDisable{}.Call(r.client)
}

// hijackHandler 处理每一个和regexp匹配的请求
type hijackHandler struct {
	pattern string
	regexp  *regexp.Regexp
	handler func(*Hijack)
}

// Hijack context
type Hijack struct {
	Request  *HijackRequest
	Response *HijackResponse
	OnError  func(error)

	// 跳过下一个 handler
	Skip bool

	continueRequest *proto.FetchContinueRequest

	// CustomState用于存储此context的内容
	CustomState interface{}

	browser *Browser
}

// ContinueRequest 不被劫持。RequestID将由router设置，你不需要设置它。
func (h *Hijack) ContinueRequest(cq *proto.FetchContinueRequest) {
	h.continueRequest = cq
}

// LoadResponse will send request to the real destination and load the response as default response to override.
// LoadResponse 将向实际目标发送请求，并将响应作为默认响应加载以覆盖。
func (h *Hijack) LoadResponse(client *http.Client, loadBody bool) error {
	res, err := client.Do(h.Request.req)
	if err != nil {
		return err
	}

	defer func() { _ = res.Body.Close() }()

	h.Response.payload.ResponseCode = res.StatusCode

	for k, vs := range res.Header {
		for _, v := range vs {
			h.Response.SetHeader(k, v)
		}
	}

	if loadBody {
		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		h.Response.payload.Body = b
	}

	return nil
}

// HijackRequest context
type HijackRequest struct {
	event *proto.FetchRequestPaused
	req   *http.Request
}

// Type of the resource
// 资源的类型
func (ctx *HijackRequest) Type() proto.NetworkResourceType {
	return ctx.event.ResourceType
}

// Method of the request
// 请求的方法
func (ctx *HijackRequest) Method() string {
	return ctx.event.Request.Method
}

// URL of the request
// 请求的URL
func (ctx *HijackRequest) URL() *url.URL {
	u, _ := url.Parse(ctx.event.Request.URL)
	return u
}

// Header via a key
// 通过key获得相应的请求头的值
func (ctx *HijackRequest) Header(key string) string {
	return ctx.event.Request.Headers[key].String()
}

// Headers of request
// 请求的请求头
func (ctx *HijackRequest) Headers() proto.NetworkHeaders {
	return ctx.event.Request.Headers
}

// Body of the request, devtools API doesn't support binary data yet, only string can be captured.
// 请求体，devtools API还不支持二进制数据，只能捕获字符串。
func (ctx *HijackRequest) Body() string {
	return ctx.event.Request.PostData
}

// JSONBody of the request
// 请求的JSONBody
func (ctx *HijackRequest) JSONBody() gson.JSON {
	return gson.NewFrom(ctx.Body())
}

// Req returns the underlaying http.Request instance that will be used to send the request.
// Req返回将用于发送请求的http.Request实例的底层。
func (ctx *HijackRequest) Req() *http.Request {
	return ctx.req
}

// SetContext of the underlaying http.Request instance
// 设置底层http.Request实例的上下文。
func (ctx *HijackRequest) SetContext(c context.Context) *HijackRequest {
	ctx.req = ctx.req.WithContext(c)
	return ctx
}

// SetBody of the request, if obj is []byte or string, raw body will be used, else it will be encoded as json.
// 设置请求的正文，如果obj是[]字节或字符串，将使用原始正文，否则将被编码为json。
func (ctx *HijackRequest) SetBody(obj interface{}) *HijackRequest {
	var b []byte

	switch body := obj.(type) {
	case []byte:
		b = body
	case string:
		b = []byte(body)
	default:
		b = utils.MustToJSONBytes(body)
	}

	ctx.req.Body = ioutil.NopCloser(bytes.NewBuffer(b))

	return ctx
}

// IsNavigation determines whether the request is a navigation request
// IsNavigation 确定请求是否是一个导航请求
func (ctx *HijackRequest) IsNavigation() bool {
	return ctx.Type() == proto.NetworkResourceTypeDocument
}

// HijackResponse context
type HijackResponse struct {
	payload *proto.FetchFulfillRequest
	fail    *proto.FetchFailRequest
}

// Payload to respond the request from the browser.
// 来自浏览器请求响应的 payload
func (ctx *HijackResponse) Payload() *proto.FetchFulfillRequest {
	return ctx.payload
}

// Body of the payload
// playload 的主体
func (ctx *HijackResponse) Body() string {
	return string(ctx.payload.Body)
}

// Headers returns the clone of response headers.
// 返回响应头的克隆
// If you want to modify the response headers use HijackResponse.SetHeader .
// 如果想修改响应头请使用：HijackResponse.SetHeader
func (ctx *HijackResponse) Headers() http.Header {
	header := http.Header{}

	for _, h := range ctx.payload.ResponseHeaders {
		header.Add(h.Name, h.Value)
	}

	return header
}

// SetHeader of the payload via key-value pairs
// 通过键值对儿为 playload 设置响应头
func (ctx *HijackResponse) SetHeader(pairs ...string) *HijackResponse {
	for i := 0; i < len(pairs); i += 2 {
		ctx.payload.ResponseHeaders = append(ctx.payload.ResponseHeaders, &proto.FetchHeaderEntry{
			Name:  pairs[i],
			Value: pairs[i+1],
		})
	}
	return ctx
}

// SetBody of the payload, if obj is []byte or string, raw body will be used, else it will be encoded as json.
// 设置有效载荷的主体，如果obj是[]字节或字符串，将使用原始主体，否则将被编码为json。
func (ctx *HijackResponse) SetBody(obj interface{}) *HijackResponse {
	switch body := obj.(type) {
	case []byte:
		ctx.payload.Body = body
	case string:
		ctx.payload.Body = []byte(body)
	default:
		ctx.payload.Body = utils.MustToJSONBytes(body)
	}
	return ctx
}

// Fail request
func (ctx *HijackResponse) Fail(reason proto.NetworkErrorReason) *HijackResponse {
	ctx.fail.ErrorReason = reason
	return ctx
}

// HandleAuth for the next basic HTTP authentication.
// HandleAuth用于下一次基本HTTP认证。
// It will prevent the popup that requires user to input user name and password.
// 它将阻止要求用户输入用户名和密码的弹出窗口。
// Ref: https://developer.mozilla.org/en-US/docs/Web/HTTP/Authentication
func (b *Browser) HandleAuth(username, password string) func() error {
	enable := b.DisableDomain("", &proto.FetchEnable{})
	disable := b.EnableDomain("", &proto.FetchEnable{
		HandleAuthRequests: true,
	})

	paused := &proto.FetchRequestPaused{}
	auth := &proto.FetchAuthRequired{}

	ctx, cancel := context.WithCancel(b.ctx)
	waitPaused := b.Context(ctx).WaitEvent(paused)
	waitAuth := b.Context(ctx).WaitEvent(auth)

	return func() (err error) {
		defer enable()
		defer disable()
		defer cancel()

		waitPaused()

		err = proto.FetchContinueRequest{
			RequestID: paused.RequestID,
		}.Call(b)
		if err != nil {
			return
		}

		waitAuth()

		err = proto.FetchContinueWithAuth{
			RequestID: auth.RequestID,
			AuthChallengeResponse: &proto.FetchAuthChallengeResponse{
				Response: proto.FetchAuthChallengeResponseResponseProvideCredentials,
				Username: username,
				Password: password,
			},
		}.Call(b)

		return
	}
}
