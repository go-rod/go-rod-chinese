package rod

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/ysmood/gson"

	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/js"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
)

// Element 实现了这些接口
var _ proto.Client = &Element{}
var _ proto.Contextable = &Element{}
var _ proto.Sessionable = &Element{}

// Element 代表DOM中的元素
type Element struct {
	Object *proto.RuntimeRemoteObject

	e eFunc

	ctx context.Context

	sleeper func() utils.Sleeper

	page *Page
}

// GetSessionID 接口
func (el *Element) GetSessionID() proto.TargetSessionID {
	return el.page.SessionID
}

// String 接口
func (el *Element) String() string {
	return fmt.Sprintf("<%s>", el.Object.Description)
}

// Page 元素所在的页面
func (el *Element) Page() *Page {
	return el.page
}

// Focus 设置指定元素的焦点
// 在执行之前，它将尝试滚动到该元素。
func (el *Element) Focus() error {
	err := el.ScrollIntoView()
	if err != nil {
		return err
	}

	_, err = el.Evaluate(Eval(`() => this.focus()`).ByUser())
	return err
}

// ScrollIntoView 将当前元素滚动到浏览器窗口的可见区域中（如果它尚未在可见区域内）。
func (el *Element) ScrollIntoView() error {
	defer el.tryTrace(TraceTypeInput, "scroll into view")()
	el.page.browser.trySlowmotion()

	err := el.WaitStableRAF()
	if err != nil {
		return err
	}

	return proto.DOMScrollIntoViewIfNeeded{ObjectID: el.id()}.Call(el)
}

// Hover 将鼠标停在元素的中心
// 在执行该操作之前，它将尝试滚动到该元素并等待其可交互。
func (el *Element) Hover() error {
	pt, err := el.WaitInteractable()
	if err != nil {
		return err
	}

	return el.page.Mouse.Move(pt.X, pt.Y, 1)
}

// MoveMouseOut 将鼠标移出当前元素
func (el *Element) MoveMouseOut() error {
	shape, err := el.Shape()
	if err != nil {
		return err
	}
	box := shape.Box()
	return el.page.Mouse.Move(box.X+box.Width, box.Y, 1)
}

// Click 会像人一样按下然后释放按钮。
// 在执行操作之前，它将尝试滚动到元素，将鼠标悬停在该元素上，等待该元素可交互并启用。
func (el *Element) Click(button proto.InputMouseButton) error {
	err := el.Hover()
	if err != nil {
		return err
	}

	err = el.WaitEnabled()
	if err != nil {
		return err
	}

	defer el.tryTrace(TraceTypeInput, string(button)+" click")()

	return el.page.Mouse.Click(button)
}

// Tap 将滚动到按钮并像人类一样点击它。
// 在执行此操作之前，它将尝试滚动到元素，并等待其可交互并启用。
func (el *Element) Tap() error {
	err := el.ScrollIntoView()
	if err != nil {
		return err
	}

	err = el.WaitEnabled()
	if err != nil {
		return err
	}

	pt, err := el.WaitInteractable()
	if err != nil {
		return err
	}

	defer el.tryTrace(TraceTypeInput, "tap")()

	return el.page.Touch.Tap(pt.X, pt.Y)
}

// Interactable 检查该元素是否可以与光标交互。
// 光标可以是鼠标、手指、手写笔等。
// 如果不是可交互的，Err将是ErrNotInteractable，例如当被一个模态框覆盖时。
func (el *Element) Interactable() (pt *proto.Point, err error) {
	noPointerEvents, err := el.Eval(`() => getComputedStyle(this).pointerEvents === 'none'`)
	if err != nil {
		return nil, err
	}

	if noPointerEvents.Value.Bool() {
		return nil, &ErrNoPointerEvents{el}
	}

	shape, err := el.Shape()
	if err != nil {
		return nil, err
	}

	pt = shape.OnePointInside()
	if pt == nil {
		err = &ErrInvisibleShape{el}
		return
	}

	scroll, err := el.page.root.Eval(`() => ({ x: window.scrollX, y: window.scrollY })`)
	if err != nil {
		return
	}

	elAtPoint, err := el.page.ElementFromPoint(
		int(pt.X)+scroll.Value.Get("x").Int(),
		int(pt.Y)+scroll.Value.Get("y").Int(),
	)
	if err != nil {
		if errors.Is(err, cdp.ErrNodeNotFoundAtPos) {
			err = &ErrInvisibleShape{el}
		}
		return
	}

	isParent, err := el.ContainsElement(elAtPoint)
	if err != nil {
		return
	}

	if !isParent {
		err = &ErrCovered{elAtPoint}
	}
	return
}

// Shape DOM元素内容的形状。该形状是一组4边多边形（4角）。
// 4-gon不一定是一个长方形。4-gon可以彼此分开。
// 例如，我们使用2个4角来描述以下形状：
//
//       ____________          ____________
//      /        ___/    =    /___________/    +     _________
//     /________/                                   /________/
//
func (el *Element) Shape() (*proto.DOMGetContentQuadsResult, error) {
	return proto.DOMGetContentQuads{ObjectID: el.id()}.Call(el)
}

// Type 与Keyboard.Type类似。
// 在执行操作之前，它将尝试滚动到该元素并将焦点集中在该元素上。
func (el *Element) Type(keys ...input.Key) error {
	err := el.Focus()
	if err != nil {
		return err
	}
	return el.page.Keyboard.Type(keys...)
}

// KeyActions 与Page.KeyActions类似。
// 在执行操作之前，它将尝试滚动到该元素并将焦点集中在该元素上。
func (el *Element) KeyActions() (*KeyActions, error) {
	err := el.Focus()
	if err != nil {
		return nil, err
	}

	return el.page.KeyActions(), nil
}

// SelectText 选择与正则表达式匹配的文本。
// 在执行操作之前，它将尝试滚动到该元素并将焦点集中在该元素上。
func (el *Element) SelectText(regex string) error {
	err := el.Focus()
	if err != nil {
		return err
	}

	defer el.tryTrace(TraceTypeInput, "select text: "+regex)()
	el.page.browser.trySlowmotion()

	_, err = el.Evaluate(evalHelper(js.SelectText, regex).ByUser())
	return err
}

// SelectAllText 选择所有文本
// 在执行操作之前，它将尝试滚动到该元素并将焦点集中在该元素上。
func (el *Element) SelectAllText() error {
	err := el.Focus()
	if err != nil {
		return err
	}

	defer el.tryTrace(TraceTypeInput, "select all text")()
	el.page.browser.trySlowmotion()

	_, err = el.Evaluate(evalHelper(js.SelectAllText).ByUser())
	return err
}

// Input 聚焦在该元素上并输入文本.
// 在执行操作之前，它将滚动到元素，等待其可见、启用和可写。
// 要清空输入，可以使用el.SelectAllText（）.MustInput（“”）之类的命令
func (el *Element) Input(text string) error {
	err := el.Focus()
	if err != nil {
		return err
	}

	err = el.WaitEnabled()
	if err != nil {
		return err
	}

	err = el.WaitWritable()
	if err != nil {
		return err
	}

	err = el.page.InsertText(text)
	_, _ = el.Evaluate(evalHelper(js.InputEvent).ByUser())
	return err
}

// InputTime 聚焦该元素及其输入时间。
// 在执行操作之前，它将滚动到元素，等待其可见、启用和可写。
// 它将等待元素可见、启用和可写。
func (el *Element) InputTime(t time.Time) error {
	err := el.Focus()
	if err != nil {
		return err
	}

	err = el.WaitEnabled()
	if err != nil {
		return err
	}

	err = el.WaitWritable()
	if err != nil {
		return err
	}

	defer el.tryTrace(TraceTypeInput, "input "+t.String())()

	_, err = el.Evaluate(evalHelper(js.InputTime, t.UnixNano()/1e6).ByUser())
	return err
}

// Blur 类似于方法 Blur
func (el *Element) Blur() error {
	_, err := el.Evaluate(Eval("() => this.blur()").ByUser())
	return err
}

// Select 选择与选择器匹配的子选项元素。
// 在操作之前，它将滚动到元素，等待它可见。
// 如果没有与选择器匹配的选项，它将返回ErrElementNotFound。
func (el *Element) Select(selectors []string, selected bool, t SelectorType) error {
	err := el.Focus()
	if err != nil {
		return err
	}

	defer el.tryTrace(TraceTypeInput, fmt.Sprintf(`select "%s"`, strings.Join(selectors, "; ")))()
	el.page.browser.trySlowmotion()

	res, err := el.Evaluate(evalHelper(js.Select, selectors, selected, t).ByUser())
	if err != nil {
		return err
	}
	if !res.Value.Bool() {
		return &ErrElementNotFound{}
	}
	return nil
}

// Matches 检查css选择器是否可以选择元素
func (el *Element) Matches(selector string) (bool, error) {
	res, err := el.Eval(`s => this.matches(s)`, selector)
	if err != nil {
		return false, err
	}
	return res.Value.Bool(), nil
}

// Attribute DOM对象的属性
// Attribute vs Property: https://stackoverflow.com/questions/6003819/what-is-the-difference-between-properties-and-attributes-in-html
func (el *Element) Attribute(name string) (*string, error) {
	attr, err := el.Eval("(n) => this.getAttribute(n)", name)
	if err != nil {
		return nil, err
	}

	if attr.Value.Nil() {
		return nil, nil
	}

	s := attr.Value.Str()
	return &s, nil
}

// Property DOM对象的属性
// Property vs Attribute: https://stackoverflow.com/questions/6003819/what-is-the-difference-between-properties-and-attributes-in-html
func (el *Element) Property(name string) (gson.JSON, error) {
	prop, err := el.Eval("(n) => this[n]", name)
	if err != nil {
		return gson.New(nil), err
	}

	return prop.Value, nil
}

// SetFiles 设置当前文件输入元素的文件
func (el *Element) SetFiles(paths []string) error {
	absPaths := []string{}
	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		utils.E(err)
		absPaths = append(absPaths, absPath)
	}

	defer el.tryTrace(TraceTypeInput, fmt.Sprintf("set files: %v", absPaths))()
	el.page.browser.trySlowmotion()

	err := proto.DOMSetFileInputFiles{
		Files:    absPaths,
		ObjectID: el.id(),
	}.Call(el)

	return err
}

// Describe 描述当前元素。深度是应检索子级的最大深度，默认为1，对整个子树使用-1，或提供大于0的整数。
// pierce决定在返回子树时是否要遍历iframes和影子根。
// 返回的proto.DOMNode。NodeID将始终为空，因为NodeID不稳定（当proto.DOMDocumentUpdated被触发时，
// 页面上的所有NodeID都将被重新分配到另一个值）。我们不建议使用NodeID，而是使用BackendNodeID来标识元素。
func (el *Element) Describe(depth int, pierce bool) (*proto.DOMNode, error) {
	val, err := proto.DOMDescribeNode{ObjectID: el.id(), Depth: gson.Int(depth), Pierce: pierce}.Call(el)
	if err != nil {
		return nil, err
	}
	return val.Node, nil
}

// ShadowRoot ShadowRoot返回此元素的影子根
func (el *Element) ShadowRoot() (*Element, error) {
	node, err := el.Describe(1, false)
	if err != nil {
		return nil, err
	}

	// 虽然现在它是一个数组，但w3c将其规范更改为单个数组。
	id := node.ShadowRoots[0].BackendNodeID

	shadowNode, err := proto.DOMResolveNode{BackendNodeID: id}.Call(el)
	if err != nil {
		return nil, err
	}

	return el.page.ElementFromObject(shadowNode.Object)
}

// Frame 创建一个表示iframe的页面实例
func (el *Element) Frame() (*Page, error) {
	node, err := el.Describe(1, false)
	if err != nil {
		return nil, err
	}

	clone := *el.page
	clone.FrameID = node.FrameID
	clone.jsCtxID = new(proto.RuntimeRemoteObjectID)
	clone.element = el
	clone.sleeper = el.sleeper

	return &clone, nil
}

// ContainesElement 检查目标是否是或在元素内。
func (el *Element) ContainsElement(target *Element) (bool, error) {
	res, err := el.Evaluate(evalHelper(js.ContainsElement, target.Object))
	if err != nil {
		return false, err
	}
	return res.Value.Bool(), nil
}

// Text 元素显示的文本
func (el *Element) Text() (string, error) {
	str, err := el.Evaluate(evalHelper(js.Text))
	if err != nil {
		return "", err
	}
	return str.Value.String(), nil
}

// HTML 元素的HTML
func (el *Element) HTML() (string, error) {
	res, err := proto.DOMGetOuterHTML{ObjectID: el.Object.ObjectID}.Call(el)
	if err != nil {
		return "", err
	}
	return res.OuterHTML, nil
}

// Visible 如果元素在页面上可见，则返回true
func (el *Element) Visible() (bool, error) {
	res, err := el.Evaluate(evalHelper(js.Visible))
	if err != nil {
		return false, err
	}
	return res.Value.Bool(), nil
}

// WaitLoad 类似于＜img＞元素的等待加载
func (el *Element) WaitLoad() error {
	defer el.tryTrace(TraceTypeWait, "load")()
	_, err := el.Evaluate(evalHelper(js.WaitLoad).ByPromise())
	return err
}

// WaitStable 等待直到在d持续时间内没有形状或位置变化。
// 小心，d不是最大等待超时，它是最不稳定的时间。
// 如果要设置超时，可以使用“Element.timeout”函数。
func (el *Element) WaitStable(d time.Duration) error {
	err := el.WaitVisible()
	if err != nil {
		return err
	}

	defer el.tryTrace(TraceTypeWait, "stable")()

	shape, err := el.Shape()
	if err != nil {
		return err
	}

	t := time.NewTicker(d)
	defer t.Stop()

	for {
		select {
		case <-t.C:
		case <-el.ctx.Done():
			return el.ctx.Err()
		}
		current, err := el.Shape()
		if err != nil {
			return err
		}
		if reflect.DeepEqual(shape, current) {
			break
		}
		shape = current
	}
	return nil
}

// WaitStableRAF 等待直到连续两个动画帧的形状或位置没有变化。
// 如果要等待由JS而不是CSS触发的动画，最好使用Element.WaitStable。
// 关于 animation frame: https://developer.mozilla.org/en-US/docs/Web/API/window/requestAnimationFrame
func (el *Element) WaitStableRAF() error {
	err := el.WaitVisible()
	if err != nil {
		return err
	}

	defer el.tryTrace(TraceTypeWait, "stable RAF")()

	var shape *proto.DOMGetContentQuadsResult

	for {
		err = el.page.WaitRepaint()
		if err != nil {
			return err
		}

		current, err := el.Shape()
		if err != nil {
			return err
		}
		if reflect.DeepEqual(shape, current) {
			break
		}
		shape = current
	}
	return nil
}

// WaitInteractable 等待元素可交互。
// 它将在每次尝试时尝试滚动到元素。
func (el *Element) WaitInteractable() (pt *proto.Point, err error) {
	defer el.tryTrace(TraceTypeWait, "interactable")()

	err = utils.Retry(el.ctx, el.sleeper(), func() (bool, error) {
		// 对于延迟加载页面，元素可以在视口之外。
		// 如果我们不滚动到它，它将永远不可用。
		err := el.ScrollIntoView()
		if err != nil {
			return true, err
		}

		pt, err = el.Interactable()
		if errors.Is(err, &ErrCovered{}) {
			return false, nil
		}
		return true, err
	})
	return
}

// Wait 等待js返回true
func (el *Element) Wait(opts *EvalOptions) error {
	return el.page.Context(el.ctx).Sleeper(el.sleeper).Wait(opts.This(el.Object))
}

// WaitVisible 直到元素可见
func (el *Element) WaitVisible() error {
	defer el.tryTrace(TraceTypeWait, "visible")()
	return el.Wait(evalHelper(js.Visible))
}

// WaitEnabled 直到该元素未被禁用。
// Doc for readonly: https://developer.mozilla.org/en-US/docs/Web/HTML/Attributes/readonly
func (el *Element) WaitEnabled() error {
	defer el.tryTrace(TraceTypeWait, "enabled")()
	return el.Wait(Eval(`() => !this.disabled`))
}

// WaitWritable 直到该元素不是只读的。
// Doc for disabled: https://developer.mozilla.org/en-US/docs/Web/HTML/Attributes/disabled
func (el *Element) WaitWritable() error {
	defer el.tryTrace(TraceTypeWait, "writable")()
	return el.Wait(Eval(`() => !this.readonly`))
}

// WaitInvisible 直到元件不可见
func (el *Element) WaitInvisible() error {
	defer el.tryTrace(TraceTypeWait, "invisible")()
	return el.Wait(evalHelper(js.Invisible))
}

// CanvastoiImage 获取画布的图像数据。
// 默认格式为image/png。
// 默认质量为0.92。
// doc: https://developer.mozilla.org/en-US/docs/Web/API/HTMLCanvasElement/toDataURL
func (el *Element) CanvasToImage(format string, quality float64) ([]byte, error) {
	res, err := el.Eval(`(format, quality) => this.toDataURL(format, quality)`, format, quality)
	if err != nil {
		return nil, err
	}

	_, bin := parseDataURI(res.Value.Str())
	return bin, nil
}

// Resource 返回当前元素的“src”内容。例如<img src=“a.jpg”>的jpg
func (el *Element) Resource() ([]byte, error) {
	src, err := el.Evaluate(evalHelper(js.Resource).ByPromise())
	if err != nil {
		return nil, err
	}

	return el.page.GetResource(src.Value.String())
}

// BackgroundImage 返回元素的css背景图像
func (el *Element) BackgroundImage() ([]byte, error) {
	res, err := el.Eval(`() => window.getComputedStyle(this).backgroundImage.replace(/^url\("/, '').replace(/"\)$/, '')`)
	if err != nil {
		return nil, err
	}

	u := res.Value.Str()

	return el.page.GetResource(u)
}

// Screenshot 元素区域的屏幕截图
func (el *Element) Screenshot(format proto.PageCaptureScreenshotFormat, quality int) ([]byte, error) {
	err := el.ScrollIntoView()
	if err != nil {
		return nil, err
	}

	opts := &proto.PageCaptureScreenshot{
		Quality: gson.Int(quality),
		Format:  format,
	}

	bin, err := el.page.Screenshot(false, opts)
	if err != nil {
		return nil, err
	}

	// 这样它就不会剪切css转换后的元素
	shape, err := el.Shape()
	if err != nil {
		return nil, err
	}

	box := shape.Box()

	// TODO: proto.PageCaptureScreenshot has a Clip option, but it's buggy, so now we do in Go.
	return utils.CropImage(bin, quality,
		int(box.X),
		int(box.Y),
		int(box.Width),
		int(box.Height),
	)
}

// Release 是Page.Release（el.Object）的快捷方式
func (el *Element) Release() error {
	return el.page.Context(el.ctx).Release(el.Object)
}

// Remove 从页面中删除元素
func (el *Element) Remove() error {
	_, err := el.Eval(`() => this.remove()`)
	if err != nil {
		return err
	}
	return el.Release()
}

// Call 实现proto.Client
func (el *Element) Call(ctx context.Context, sessionID, methodName string, params interface{}) (res []byte, err error) {
	return el.page.Call(ctx, sessionID, methodName, params)
}

// Eval 是Element.Evaluate的一个快捷方式，其中AwaitPromise、ByValue和AutoExp设置为 "true"。
func (el *Element) Eval(js string, params ...interface{}) (*proto.RuntimeRemoteObject, error) {
	return el.Evaluate(Eval(js, params...).ByPromise())
}

// Evaluate 只是Page.Evaluate的一个快捷方式，This设置为当前元素。
func (el *Element) Evaluate(opts *EvalOptions) (*proto.RuntimeRemoteObject, error) {
	return el.page.Context(el.ctx).Evaluate(opts.This(el.Object))
}

// Equal 检查两个元素是否相等。
func (el *Element) Equal(elm *Element) (bool, error) {
	res, err := el.Eval(`elm => this === elm`, elm.Object)
	return res.Value.Bool(), err
}

func (el *Element) id() proto.RuntimeRemoteObjectID {
	return el.Object.ObjectID
}
