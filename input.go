package rod

import (
	"fmt"
	"sync"

	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
	"github.com/ysmood/gson"
)

// Keyboard represents the keyboard on a page, it's always related the main frame
// keyboard 代表一个页面上的键盘，它总是与主frame相关
type Keyboard struct {
	sync.Mutex

	page *Page

	// pressed keys must be released before it can be pressed again
	// 必须释放以后，才能再次按下按键
	pressed map[input.Key]struct{}
}

func (p *Page) newKeyboard() *Page {
	p.Keyboard = &Keyboard{page: p, pressed: map[input.Key]struct{}{}}
	return p
}

func (k *Keyboard) getModifiers() int {
	k.Lock()
	defer k.Unlock()
	return k.modifiers()
}

func (k *Keyboard) modifiers() int {
	ms := 0
	for key := range k.pressed {
		ms |= key.Modifier()
	}
	return ms
}

// Press the key down.
// To input characters that are not on the keyboard, such as Chinese or Japanese, you should
// use method like Page.InsertText .
// 按下按键
// 要输入键盘上没有的字符，如中文或日文，你应该使用类似Page.InsertText的方法。
func (k *Keyboard) Press(key input.Key) error {
	defer k.page.tryTrace(TraceTypeInput, "press key: "+key.Info().Code)()
	k.page.browser.trySlowmotion()

	k.Lock()
	defer k.Unlock()

	k.pressed[key] = struct{}{}

	return key.Encode(proto.InputDispatchKeyEventTypeKeyDown, k.modifiers()).Call(k.page)
}

// Release the key
// 释放按键
func (k *Keyboard) Release(key input.Key) error {
	defer k.page.tryTrace(TraceTypeInput, "release key: "+key.Info().Code)()

	k.Lock()
	defer k.Unlock()

	if _, has := k.pressed[key]; !has {
		return nil
	}

	delete(k.pressed, key)

	return key.Encode(proto.InputDispatchKeyEventTypeKeyUp, k.modifiers()).Call(k.page)
}

// Type releases the key after the press
// 按下后紧接着释放
func (k *Keyboard) Type(keys ...input.Key) (err error) {
	for _, key := range keys {
		err = k.Press(key)
		if err != nil {
			return
		}
		err = k.Release(key)
		if err != nil {
			return
		}
	}
	return
}

// KeyActionType enum
// 枚举 KeyActionType
type KeyActionType int

// KeyActionTypes
const (
	KeyActionPress KeyActionType = iota
	KeyActionRelease
	KeyActionTypeKey
)

// KeyAction to perform
// 执行按键操作
type KeyAction struct {
	Type KeyActionType
	Key  input.Key
}

// KeyActions to simulate
// 模拟按键操作
type KeyActions struct {
	keyboard *Keyboard

	Actions []KeyAction
}

// KeyActions simulates the type actions on a physical keyboard.
// Useful when input shortcuts like ctrl+enter .
// KeyActions 模拟物理键盘上的类型操作。
// 尤其在执行像 ctrl+enter 快捷键时非常有用
func (p *Page) KeyActions() *KeyActions {
	return &KeyActions{keyboard: p.Keyboard}
}

// Press keys is guaranteed to have a release at the end of actions
// 用来确保每次操作结束后释放，再按下
func (ka *KeyActions) Press(keys ...input.Key) *KeyActions {
	for _, key := range keys {
		ka.Actions = append(ka.Actions, KeyAction{KeyActionPress, key})
	}
	return ka
}

// Release keys
// 释放按键
func (ka *KeyActions) Release(keys ...input.Key) *KeyActions {
	for _, key := range keys {
		ka.Actions = append(ka.Actions, KeyAction{KeyActionRelease, key})
	}
	return ka
}

// Type will release the key immediately after the pressing
// Type 会立即释放按键
func (ka *KeyActions) Type(keys ...input.Key) *KeyActions {
	for _, key := range keys {
		ka.Actions = append(ka.Actions, KeyAction{KeyActionTypeKey, key})
	}
	return ka
}

// Do the actions
// 执行相应的按键操作
func (ka *KeyActions) Do() (err error) {
	for _, a := range ka.balance() {
		switch a.Type {
		case KeyActionPress:
			err = ka.keyboard.Press(a.Key)
		case KeyActionRelease:
			err = ka.keyboard.Release(a.Key)
		case KeyActionTypeKey:
			err = ka.keyboard.Type(a.Key)
		}
		if err != nil {
			return
		}
	}
	return
}

// Make sure there's at least one release after the presses, such as:
// 确保按下后至少有一次释放
//     p1,p2,p1,r1 => p1,p2,p1,r1,r2
func (ka *KeyActions) balance() []KeyAction {
	actions := ka.Actions

	h := map[input.Key]bool{}
	for _, a := range actions {
		switch a.Type {
		case KeyActionPress:
			h[a.Key] = true
		case KeyActionRelease, KeyActionTypeKey:
			h[a.Key] = false
		}
	}

	for key, needRelease := range h {
		if needRelease {
			actions = append(actions, KeyAction{KeyActionRelease, key})
		}
	}

	return actions
}

// InsertText is like pasting text into the page
// 类似于将文本粘贴到页面中
func (p *Page) InsertText(text string) error {
	defer p.tryTrace(TraceTypeInput, "insert text "+text)()
	p.browser.trySlowmotion()

	err := proto.InputInsertText{Text: text}.Call(p)
	return err
}

// Mouse represents the mouse on a page, it's always related the main frame
// 代表一个在页面中的鼠标，总是依赖于主frame
type Mouse struct {
	sync.Mutex

	page *Page

	id string // 鼠标所在的SVG DOM元素的ID

	x float64
	y float64

	// the buttons is currently being pressed, reflects the press order
	// 目前正在被按下的按钮，反映了被按下的顺序
	buttons []proto.InputMouseButton
}

func (p *Page) newMouse() *Page {
	p.Mouse = &Mouse{page: p, id: utils.RandString(8)}
	return p
}

// Move to the absolute position with specified steps
// 以指定的步骤移动到绝对位置
func (m *Mouse) Move(x, y float64, steps int) error {
	m.Lock()
	defer m.Unlock()

	if steps < 1 {
		steps = 1
	}

	stepX := (x - m.x) / float64(steps)
	stepY := (y - m.y) / float64(steps)

	button, buttons := input.EncodeMouseButton(m.buttons)

	for i := 0; i < steps; i++ {
		m.page.browser.trySlowmotion()

		toX := m.x + stepX
		toY := m.y + stepY

		err := proto.InputDispatchMouseEvent{
			Type:      proto.InputDispatchMouseEventTypeMouseMoved,
			X:         toX,
			Y:         toY,
			Button:    button,
			Buttons:   gson.Int(buttons),
			Modifiers: m.page.Keyboard.getModifiers(),
		}.Call(m.page)
		if err != nil {
			return err
		}

		// to make sure set only when call is successful
		// 确保被调用成功时才会被设置
		m.x = toX
		m.y = toY

		if m.page.browser.trace {
			if !m.updateMouseTracer() {
				m.initMouseTracer()
				m.updateMouseTracer()
			}
		}
	}

	return nil
}

// Scroll the relative offset with specified steps
// 以指定的步骤滚动相对偏移量
func (m *Mouse) Scroll(offsetX, offsetY float64, steps int) error {
	m.Lock()
	defer m.Unlock()

	defer m.page.tryTrace(TraceTypeInput, fmt.Sprintf("scroll (%.2f, %.2f)", offsetX, offsetY))()
	m.page.browser.trySlowmotion()

	if steps < 1 {
		steps = 1
	}

	button, buttons := input.EncodeMouseButton(m.buttons)

	stepX := offsetX / float64(steps)
	stepY := offsetY / float64(steps)

	for i := 0; i < steps; i++ {
		err := proto.InputDispatchMouseEvent{
			Type:      proto.InputDispatchMouseEventTypeMouseWheel,
			X:         m.x,
			Y:         m.y,
			Button:    button,
			Buttons:   gson.Int(buttons),
			Modifiers: m.page.Keyboard.getModifiers(),
			DeltaX:    stepX,
			DeltaY:    stepY,
		}.Call(m.page)
		if err != nil {
			return err
		}
	}

	return nil
}

// Down holds the button down
// 向下按住按钮
func (m *Mouse) Down(button proto.InputMouseButton, clicks int) error {
	m.Lock()
	defer m.Unlock()

	toButtons := append(m.buttons, button)

	_, buttons := input.EncodeMouseButton(toButtons)

	err := proto.InputDispatchMouseEvent{
		Type:       proto.InputDispatchMouseEventTypeMousePressed,
		Button:     button,
		Buttons:    gson.Int(buttons),
		ClickCount: clicks,
		Modifiers:  m.page.Keyboard.getModifiers(),
		X:          m.x,
		Y:          m.y,
	}.Call(m.page)
	if err != nil {
		return err
	}
	m.buttons = toButtons
	return nil
}

// Up releases the button
// 向上释放按钮
func (m *Mouse) Up(button proto.InputMouseButton, clicks int) error {
	m.Lock()
	defer m.Unlock()

	toButtons := []proto.InputMouseButton{}
	for _, btn := range m.buttons {
		if btn == button {
			continue
		}
		toButtons = append(toButtons, btn)
	}

	_, buttons := input.EncodeMouseButton(toButtons)

	err := proto.InputDispatchMouseEvent{
		Type:       proto.InputDispatchMouseEventTypeMouseReleased,
		Button:     button,
		Buttons:    gson.Int(buttons),
		ClickCount: clicks,
		Modifiers:  m.page.Keyboard.getModifiers(),
		X:          m.x,
		Y:          m.y,
	}.Call(m.page)
	if err != nil {
		return err
	}
	m.buttons = toButtons
	return nil
}

// Click the button. It's the combination of Mouse.Down and Mouse.Up
// 点击按钮。它是Mouse.Down和Mouse.Up的组合。
func (m *Mouse) Click(button proto.InputMouseButton) error {
	m.page.browser.trySlowmotion()

	err := m.Down(button, 1)
	if err != nil {
		return err
	}

	return m.Up(button, 1)
}

// Touch presents a touch device, such as a hand with fingers, each finger is a proto.InputTouchPoint.
// Touch events is stateless, we use the struct here only as a namespace to make the API style unified.
// Touch 代表一个触摸设备，例如带手指的手，每个手指都是一个原型输入点。
// Touch 事件是无状态的，我们在这里只把结构作为命名空间，使API的风格统一。
type Touch struct {
	page *Page
}

func (p *Page) newTouch() *Page {
	p.Touch = &Touch{page: p}
	return p
}

// Start a touch action
// 开始一个触摸操作
func (t *Touch) Start(points ...*proto.InputTouchPoint) error {
	// TODO: https://crbug.com/613219
	_ = t.page.WaitRepaint()
	_ = t.page.WaitRepaint()

	return proto.InputDispatchTouchEvent{
		Type:        proto.InputDispatchTouchEventTypeTouchStart,
		TouchPoints: points,
		Modifiers:   t.page.Keyboard.getModifiers(),
	}.Call(t.page)
}

// Move touch points. Use the InputTouchPoint.ID (Touch.identifier) to track points.
// 移动触摸点。使用InputTouchPoint.ID（Touch.identifier）来跟踪点。
// Doc: https://developer.mozilla.org/en-US/docs/Web/API/Touch_events
func (t *Touch) Move(points ...*proto.InputTouchPoint) error {
	return proto.InputDispatchTouchEvent{
		Type:        proto.InputDispatchTouchEventTypeTouchMove,
		TouchPoints: points,
		Modifiers:   t.page.Keyboard.getModifiers(),
	}.Call(t.page)
}

// End touch action
// 结束触摸操作
func (t *Touch) End() error {
	return proto.InputDispatchTouchEvent{
		Type:        proto.InputDispatchTouchEventTypeTouchEnd,
		TouchPoints: []*proto.InputTouchPoint{},
		Modifiers:   t.page.Keyboard.getModifiers(),
	}.Call(t.page)
}

// Cancel touch action
// 取消触摸操作
func (t *Touch) Cancel() error {
	return proto.InputDispatchTouchEvent{
		Type:        proto.InputDispatchTouchEventTypeTouchCancel,
		TouchPoints: []*proto.InputTouchPoint{},
		Modifiers:   t.page.Keyboard.getModifiers(),
	}.Call(t.page)
}

// Tap dispatches a touchstart and touchend event.
// Tap 触发一个 touchstart 和 touchend 事件
func (t *Touch) Tap(x, y float64) error {
	defer t.page.tryTrace(TraceTypeInput, "touch")()
	t.page.browser.trySlowmotion()

	p := &proto.InputTouchPoint{X: x, Y: y}

	err := t.Start(p)
	if err != nil {
		return err
	}

	return t.End()
}
