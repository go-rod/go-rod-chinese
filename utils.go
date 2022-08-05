package rod

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime/debug"
	"sync"
	"time"

	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
)

// CDPClient 通常被用来使rod不受副作用影响。例如，代理所有rod的IO。
type CDPClient interface {
	Event() <-chan *cdp.Event
	Call(ctx context.Context, sessionID, method string, params interface{}) ([]byte, error)
}

// Message 代表一个cdp.Event
type Message struct {
	SessionID proto.TargetSessionID
	Method    string

	lock  *sync.Mutex
	data  json.RawMessage
	event reflect.Value
}

// 将数据加载到 e 中，如果 e 符合事件类型，则返回 true。
func (msg *Message) Load(e proto.Event) bool {
	if msg.Method != e.ProtoEvent() {
		return false
	}

	eVal := reflect.ValueOf(e)
	if eVal.Kind() != reflect.Ptr {
		return true
	}
	eVal = reflect.Indirect(eVal)

	msg.lock.Lock()
	defer msg.lock.Unlock()
	if msg.data == nil {
		eVal.Set(msg.event)
		return true
	}

	utils.E(json.Unmarshal(msg.data, e))
	msg.event = eVal
	msg.data = nil
	return true
}

// rod的默认Logger
var DefaultLogger = log.New(os.Stdout, "[rod] ", log.LstdFlags)

// DefaultSleeper为重试生成默认的睡眠器，它使用backoff来增长间隔时间。
// 增长情况如下:
//     A(0) = 100ms, A(n) = A(n-1) * random[1.9, 2.1), A(n) < 1s
// 为什么默认值不是RequestAnimationFrame或DOM更改事件，是因为如果重试从未结束，它很容易淹没程序。但您可以随时轻松地将其配置为所需内容。
var DefaultSleeper = func() utils.Sleeper {
	return utils.BackoffSleeper(100*time.Millisecond, time.Second, nil)
}

// PagePool以线程安全的方式限制同一时间内的页面数量。
// 使用通道来限制并发性是一种常见的做法，对于rod来说并不特殊。
// 这个helper程序更像是一个使用Go Channel的例子。
// 参考: https://golang.org/doc/effective_go#channels
type PagePool chan *Page

// NewPagePool实例
func NewPagePool(limit int) PagePool {
	pp := make(chan *Page, limit)
	for i := 0; i < limit; i++ {
		pp <- nil
	}
	return pp
}

// 从池中获取一个页面。使用PagePool.Put来使它以后可以重复使用。
func (pp PagePool) Get(create func() *Page) *Page {
	p := <-pp
	if p == nil {
		p = create()
	}
	return p
}

// 把一个页面放回池中
func (pp PagePool) Put(p *Page) {
	pp <- p
}

// 清理 helper
func (pp PagePool) Cleanup(iteratee func(*Page)) {
	for i := 0; i < cap(pp); i++ {
		p := <-pp
		if p != nil {
			iteratee(p)
		}
	}
}

// 浏览器池（BrowserPool）以线程安全的方式限制同一时间内的浏览器数量。
// 使用通道来限制并发性是一种常见的做法，这对rod来说并不特别。
// 这个helper程序更像是一个使用Go Channel的例子。
// 参考: https://golang.org/doc/effective_go#channels
type BrowserPool chan *Browser

// NewBrowserPool 实例
func NewBrowserPool(limit int) BrowserPool {
	pp := make(chan *Browser, limit)
	for i := 0; i < limit; i++ {
		pp <- nil
	}
	return pp
}

// 从池中获取一个浏览器。使用BrowserPool.Put来使它以后可以重复使用。
func (bp BrowserPool) Get(create func() *Browser) *Browser {
	p := <-bp
	if p == nil {
		p = create()
	}
	return p
}

// 将一个浏览器放回池中
func (bp BrowserPool) Put(p *Browser) {
	bp <- p
}

// 清理 helper
func (bp BrowserPool) Cleanup(iteratee func(*Browser)) {
	for i := 0; i < cap(bp); i++ {
		p := <-bp
		if p != nil {
			iteratee(p)
		}
	}
}

var _ io.ReadCloser = &StreamReader{}

// 浏览器数据流的StreamReader
type StreamReader struct {
	Offset *int

	c      proto.Client
	handle proto.IOStreamHandle
	buf    *bytes.Buffer
}

// NewStreamReader实例
func NewStreamReader(c proto.Client, h proto.IOStreamHandle) *StreamReader {
	return &StreamReader{
		c:      c,
		handle: h,
		buf:    &bytes.Buffer{},
	}
}

func (sr *StreamReader) Read(p []byte) (n int, err error) {
	res, err := proto.IORead{
		Handle: sr.handle,
		Offset: sr.Offset,
	}.Call(sr.c)
	if err != nil {
		return 0, err
	}

	if !res.EOF {
		var bin []byte
		if res.Base64Encoded {
			bin, err = base64.StdEncoding.DecodeString(res.Data)
			if err != nil {
				return 0, err
			}
		} else {
			bin = []byte(res.Data)
		}

		_, _ = sr.buf.Write(bin)
	}

	return sr.buf.Read(p)
}

// 关闭流，丢弃任何临时性的备份存储。
func (sr *StreamReader) Close() error {
	return proto.IOClose{Handle: sr.handle}.Call(sr.c)
}

// 试着用recover来尝试fn，将panic作为rod.ErrTry返回。
func Try(fn func()) (err error) {
	defer func() {
		if val := recover(); val != nil {
			err = &ErrTry{val, string(debug.Stack())}
		}
	}()

	fn()

	return err
}

func genRegMatcher(includes, excludes []string) func(string) bool {
	regIncludes := make([]*regexp.Regexp, len(includes))
	for i, p := range includes {
		regIncludes[i] = regexp.MustCompile(p)
	}

	regExcludes := make([]*regexp.Regexp, len(excludes))
	for i, p := range excludes {
		regExcludes[i] = regexp.MustCompile(p)
	}

	return func(s string) bool {
		for _, include := range regIncludes {
			if include.MatchString(s) {
				for _, exclude := range regExcludes {
					if exclude.MatchString(s) {
						goto end
					}
				}
				return true
			}
		}
	end:
		return false
	}
}

type saveFileType int

const (
	saveFileTypeScreenshot saveFileType = iota
	saveFileTypePDF
)

func saveFile(fileType saveFileType, bin []byte, toFile []string) error {
	if len(toFile) == 0 {
		return nil
	}
	if toFile[0] == "" {
		stamp := fmt.Sprintf("%d", time.Now().UnixNano())
		switch fileType {
		case saveFileTypeScreenshot:
			toFile = []string{"tmp", "screenshots", stamp + ".png"}
		case saveFileTypePDF:
			toFile = []string{"tmp", "pdf", stamp + ".pdf"}
		}
	}
	return utils.OutputFile(filepath.Join(toFile...), bin)
}

func httHTML(w http.ResponseWriter, body string) {
	w.Header().Add("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func mustToJSONForDev(value interface{}) string {
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)

	utils.E(enc.Encode(value))

	return buf.String()
}

// https://developer.mozilla.org/en-US/docs/Web/HTTP/Basics_of_HTTP/Data_URIs
var regDataURI = regexp.MustCompile(`\Adata:(.+?)?(;base64)?,`)

func parseDataURI(uri string) (string, []byte) {
	matches := regDataURI.FindStringSubmatch(uri)
	l := len(matches[0])
	contentType := matches[1]

	bin, _ := base64.StdEncoding.DecodeString(uri[l:])
	return contentType, bin
}
