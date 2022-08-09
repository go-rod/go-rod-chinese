// Package proto is a lib to encode/decode the data of the cdp protocol.
// Package proto是对cdp协议的数据进行编码/解码的库。
package proto

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
)

// Client interface to send the request.
// 用于发送请求的客户端接口
// So that this lib doesn't handle anything has side effect.
// 所以这个库不会处理任何有副作用的东西。
type Client interface {
	Call(ctx context.Context, sessionID, methodName string, params interface{}) (res []byte, err error)
}

// Sessionable type has a proto.TargetSessionID for its methods
// Sessionable 有一个 proto.TargetSessionID 方法
type Sessionable interface {
	GetSessionID() TargetSessionID
}

// Contextable type has a context.Context for its methods
// Contextable 有一个 context.Context 方法
type Contextable interface {
	GetContext() context.Context
}

// Request represents a cdp.Request.Method
// 代表一个cdp.Request.Method
type Request interface {
	// ProtoReq returns the cdp.Request.Method
	// 返回 cdp.Request.Method
	ProtoReq() string
}

// Event represents a cdp.Event.Params
// Event 代表 cdp.Event.Params
type Event interface {
	// ProtoEvent returns the cdp.Event.Method
	ProtoEvent() string
}

// GetType from method name of this package,
// such as proto.GetType("Page.enable") will return the type of proto.PageEnable
// 从这个包的方法名中获取类型，例如proto.GetType("Page.enable")将返回proto.PageEnable的类型。
func GetType(methodName string) reflect.Type {
	return types[methodName]
}

// ParseMethodName to domain and name
// 解析方法的 domain 和 name
func ParseMethodName(method string) (domain, name string) {
	arr := strings.Split(method, ".")
	return arr[0], arr[1]
}

// call method with request and response containers.
// 具有请求和响应容器的调用方式。
func call(method string, req, res interface{}, c Client) error {
	ctx := context.Background()
	if cta, ok := c.(Contextable); ok {
		ctx = cta.GetContext()
	}

	sessionID := ""
	if tsa, ok := c.(Sessionable); ok {
		sessionID = string(tsa.GetSessionID())
	}

	bin, err := c.Call(ctx, sessionID, method, req)
	if err != nil {
		return err
	}
	if res == nil {
		return nil
	}
	return json.Unmarshal(bin, res)
}
