// This file contains all query related code for Page and Element to separate the concerns.
// 该文件包含页面和元素的所有查询相关代码，用于分离关注点。

package rod

import (
	"errors"
	"regexp"

	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/js"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
)

// SelectorType enum
// 枚举选择器的类型
type SelectorType string

const (
	// SelectorTypeRegex type
	SelectorTypeRegex SelectorType = "regex"
	// SelectorTypeCSSSector type
	SelectorTypeCSSSector SelectorType = "css-selector"
	// SelectorTypeText type
	SelectorTypeText SelectorType = "text"
)

// Elements provides some helpers to deal with element list
// Elements 提供了一些帮助工具来处理元素列表
type Elements []*Element

// First returns the first element, if the list is empty returns nil
// First 返回第一个元素，如果元素列表是空的，则返回nil
func (els Elements) First() *Element {
	if els.Empty() {
		return nil
	}
	return els[0]
}

// Last returns the last element, if the list is empty returns nil
// Last 返回最后一个元素，如果元素列表是空的，则返回nil
func (els Elements) Last() *Element {
	if els.Empty() {
		return nil
	}
	return els[len(els)-1]
}

// Empty returns true if the list is empty
// 判断 Elements 是否为空
func (els Elements) Empty() bool {
	return len(els) == 0
}

// Pages provides some helpers to deal with page list
// Pages 提供了一些帮助工具来处理页面列表
type Pages []*Page

// First returns the first page, if the list is empty returns nil
// First 返回第一个页面，如果页面列表是空的，则返回nil
func (ps Pages) First() *Page {
	if ps.Empty() {
		return nil
	}
	return ps[0]
}

// Last returns the last page, if the list is empty returns nil
// Last 返回最后一个页面，如果页面列表是空的，则返回nil
func (ps Pages) Last() *Page {
	if ps.Empty() {
		return nil
	}
	return ps[len(ps)-1]
}

// Empty returns true if the list is empty
// 判断 Pages 是否为空
func (ps Pages) Empty() bool {
	return len(ps) == 0
}

// Find the page that has the specified element with the css selector
// 根据 css selector 在页面中查照指定CSS选择器的元素
func (ps Pages) Find(selector string) (*Page, error) {
	for _, page := range ps {
		has, _, err := page.Has(selector)
		if err != nil {
			return nil, err
		}
		if has {
			return page, nil
		}
	}
	return nil, &ErrPageNotFound{}
}

// FindByURL returns the page that has the url that matches the jsRegex
// 返回具有匹配jsRegex的url的页面
func (ps Pages) FindByURL(jsRegex string) (*Page, error) {
	for _, page := range ps {
		res, err := page.Eval(`() => location.href`)
		if err != nil {
			return nil, err
		}
		url := res.Value.String()
		if regexp.MustCompile(jsRegex).MatchString(url) {
			return page, nil
		}
	}
	return nil, &ErrPageNotFound{}
}

// Has an element that matches the css selector
// 在页面中用css selector查找某个元素是否存在
func (p *Page) Has(selector string) (bool, *Element, error) {
	el, err := p.Sleeper(NotFoundSleeper).Element(selector)
	if errors.Is(err, &ErrElementNotFound{}) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	return true, el.Sleeper(p.sleeper), nil
}

// HasX an element that matches the XPath selector
// 在页面中用XPath查找某个元素是否存在
func (p *Page) HasX(selector string) (bool, *Element, error) {
	el, err := p.Sleeper(NotFoundSleeper).ElementX(selector)
	if errors.Is(err, &ErrElementNotFound{}) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	return true, el.Sleeper(p.sleeper), nil
}

// HasR an element that matches the css selector and its display text matches the jsRegex.
// 在页面中用css选择器匹配，其显示文本与jsRegex匹配，使用组合的方式查找某个元素是否存在。
func (p *Page) HasR(selector, jsRegex string) (bool, *Element, error) {
	el, err := p.Sleeper(NotFoundSleeper).ElementR(selector, jsRegex)
	if errors.Is(err, &ErrElementNotFound{}) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	return true, el.Sleeper(p.sleeper), nil
}

// Element retries until an element in the page that matches the CSS selector, then returns
// the matched element.
// Element 会重试，直到页面中的元素与CSS选择器匹配，然后返回匹配的元素。
func (p *Page) Element(selector string) (*Element, error) {
	return p.ElementByJS(evalHelper(js.Element, selector))
}

// ElementR retries until an element in the page that matches the css selector and it's text matches the jsRegex,
// then returns the matched element.
// ElementR 会重试，直到页面中出现符合css选择器的元素，并且其文本符合jsRegex，然后返回匹配的元素。
func (p *Page) ElementR(selector, jsRegex string) (*Element, error) {
	return p.ElementByJS(evalHelper(js.ElementR, selector, jsRegex))
}

// ElementX retries until an element in the page that matches one of the XPath selectors, then returns
// the matched element.
// ElementX 会重试，直到页面中的元素与XPath选择器匹配，然后返回匹配的元素。
func (p *Page) ElementX(xPath string) (*Element, error) {
	return p.ElementByJS(evalHelper(js.ElementX, xPath))
}

// ElementByJS returns the element from the return value of the js function.
// ElementByJS 从js函数的返回值返回元素。
// If sleeper is nil, no retry will be performed.
// 如果 sleeper 是 nil,则不会执行重试
// By default, it will retry until the js function doesn't return null.
// 默认情况下，会一直重试直到 JS 函数不会返回 null
// To customize the retry logic, check the examples of Page.Sleeper.
// 要自定义重试逻辑，请查看 Page.Sleeper 的示例。
func (p *Page) ElementByJS(opts *EvalOptions) (*Element, error) {
	var res *proto.RuntimeRemoteObject
	var err error

	removeTrace := func() {}
	err = utils.Retry(p.ctx, p.sleeper(), func() (bool, error) {
		remove := p.tryTraceQuery(opts)
		removeTrace()
		removeTrace = remove

		res, err = p.Evaluate(opts.ByObject())
		if err != nil {
			return true, err
		}

		if res.Type == proto.RuntimeRemoteObjectTypeObject && res.Subtype == proto.RuntimeRemoteObjectSubtypeNull {
			return false, nil
		}

		return true, nil
	})
	removeTrace()
	if err != nil {
		return nil, err
	}

	if res.Subtype != proto.RuntimeRemoteObjectSubtypeNode {
		return nil, &ErrExpectElement{res}
	}

	return p.ElementFromObject(res)
}

// Elements returns all elements that match the css selector
// 返回和 CSS 选择器匹配的所有元素
func (p *Page) Elements(selector string) (Elements, error) {
	return p.ElementsByJS(evalHelper(js.Elements, selector))
}

// ElementsX returns all elements that match the XPath selector
// 返回和 XPath 选择器匹配的所有元素
func (p *Page) ElementsX(xpath string) (Elements, error) {
	return p.ElementsByJS(evalHelper(js.ElementsX, xpath))
}

// ElementsByJS returns the elements from the return value of the js
// ElementsByJS 从 js 的返回值中返回元素。
func (p *Page) ElementsByJS(opts *EvalOptions) (Elements, error) {
	res, err := p.Evaluate(opts.ByObject())
	if err != nil {
		return nil, err
	}

	if res.Subtype != proto.RuntimeRemoteObjectSubtypeArray {
		return nil, &ErrExpectElements{res}
	}

	defer func() { err = p.Release(res) }()

	list, err := proto.RuntimeGetProperties{
		ObjectID:      res.ObjectID,
		OwnProperties: true,
	}.Call(p)
	if err != nil {
		return nil, err
	}

	elemList := Elements{}
	for _, obj := range list.Result {
		if obj.Name == "__proto__" || obj.Name == "length" {
			continue
		}
		val := obj.Value

		if val.Subtype != proto.RuntimeRemoteObjectSubtypeNode {
			return nil, &ErrExpectElements{val}
		}

		el, err := p.ElementFromObject(val)
		if err != nil {
			return nil, err
		}

		elemList = append(elemList, el)
	}

	return elemList, err
}

// Search for the given query in the DOM tree until the result count is not zero, before that it will keep retrying.
// 在DOM树中搜索给定的查询，直到结果计数不为零，在此之前，它将不断重试。
// The query can be plain text or css selector or xpath.
// 查询可以是纯文本、css选择器或xpath。
// It will search nested iframes and shadow doms too.
// 它也会搜索嵌套的iframes和影子dom。
func (p *Page) Search(query string) (*SearchResult, error) {
	sr := &SearchResult{
		page:    p,
		restore: p.EnableDomain(proto.DOMEnable{}),
	}

	err := utils.Retry(p.ctx, p.sleeper(), func() (bool, error) {
		if sr.DOMPerformSearchResult != nil {
			_ = proto.DOMDiscardSearchResults{SearchID: sr.SearchID}.Call(p)
		}

		res, err := proto.DOMPerformSearch{
			Query:                     query,
			IncludeUserAgentShadowDOM: true,
		}.Call(p)
		if err != nil {
			return true, err
		}

		sr.DOMPerformSearchResult = res

		if res.ResultCount == 0 {
			return false, nil
		}

		result, err := proto.DOMGetSearchResults{
			SearchID:  res.SearchID,
			FromIndex: 0,
			ToIndex:   1,
		}.Call(p)
		if err != nil {
			// when the page is still loading the search result is not ready
			// 当页面仍然在加载时，此时的搜索结果尚未准备好。
			if errors.Is(err, cdp.ErrCtxNotFound) ||
				errors.Is(err, cdp.ErrSearchSessionNotFound) {
				return false, nil
			}
			return true, err
		}

		id := result.NodeIds[0]

		// TODO: This is definitely a bad design of cdp, hope they can optimize it in the future.
		// It's unnecessary to ask the user to explicitly call it.
		// 没有必要要求用户明确地调用它。
		//
		// When the id is zero, it means the proto.DOMDocumentUpdated has fired which will
		// invlidate all the existing NodeID. We have to call proto.DOMGetDocument
		// to reset the remote browser's tracker.
		// 当id为0时，意味着proto.DOMDocumentUpdated已经触发，它将调用所有现有的NodeID。
		// 我们必须调用proto.DOMGetDocument来重置远程浏览器的跟踪器。
		if id == 0 {
			_, _ = proto.DOMGetDocument{}.Call(p)
			return false, nil
		}

		el, err := p.ElementFromNode(&proto.DOMNode{NodeID: id})
		if err != nil {
			return true, err
		}

		sr.First = el

		return true, nil
	})
	if err != nil {
		return nil, err
	}

	return sr, nil
}

// SearchResult handler
type SearchResult struct {
	*proto.DOMPerformSearchResult

	page    *Page
	restore func()

	// First element in the search result
	// 在搜索结果中的第一个元素
	First *Element
}

// Get l elements at the index of i from the remote search result.
// 从远程搜索结果中获取索引为i的l个元素。
func (s *SearchResult) Get(i, l int) (Elements, error) {
	result, err := proto.DOMGetSearchResults{
		SearchID:  s.SearchID,
		FromIndex: i,
		ToIndex:   i + l,
	}.Call(s.page)
	if err != nil {
		return nil, err
	}

	list := Elements{}

	for _, id := range result.NodeIds {
		el, err := s.page.ElementFromNode(&proto.DOMNode{NodeID: id})
		if err != nil {
			return nil, err
		}
		list = append(list, el)
	}

	return list, nil
}

// All returns all elements
// 返回所有元素
func (s *SearchResult) All() (Elements, error) {
	return s.Get(0, s.ResultCount)
}

// Release the remote search result
// 释放搜索结果
func (s *SearchResult) Release() {
	s.restore()
	_ = proto.DOMDiscardSearchResults{SearchID: s.SearchID}.Call(s.page)
}

type raceBranch struct {
	condition func(*Page) (*Element, error)
	callback  func(*Element) error
}

// RaceContext stores the branches to race
// 存储了 race 的分支
type RaceContext struct {
	page     *Page
	branches []*raceBranch
}

// Race creates a context to race selectors
// 为 race 选择器创建了一个 RaceContext
func (p *Page) Race() *RaceContext {
	return &RaceContext{page: p}
}

// Element the doc is similar to MustElement
// 类似于 MustElement
func (rc *RaceContext) Element(selector string) *RaceContext {
	rc.branches = append(rc.branches, &raceBranch{
		condition: func(p *Page) (*Element, error) { return p.Element(selector) },
	})
	return rc
}

// ElementFunc takes a custom function to determine race success
// ElementFunc 采用自定义函数确定 race 成功
func (rc *RaceContext) ElementFunc(fn func(*Page) (*Element, error)) *RaceContext {
	rc.branches = append(rc.branches, &raceBranch{
		condition: fn,
	})
	return rc
}

// ElementX the doc is similar to ElementX
// 类似于 ElementX
func (rc *RaceContext) ElementX(selector string) *RaceContext {
	rc.branches = append(rc.branches, &raceBranch{
		condition: func(p *Page) (*Element, error) { return p.ElementX(selector) },
	})
	return rc
}

// ElementR the doc is similar to ElementR
//类似于 ElementR
func (rc *RaceContext) ElementR(selector, regex string) *RaceContext {
	rc.branches = append(rc.branches, &raceBranch{
		condition: func(p *Page) (*Element, error) { return p.ElementR(selector, regex) },
	})
	return rc
}

// ElementByJS the doc is similar to MustElementByJS
// 类似于 MustELementByJS
func (rc *RaceContext) ElementByJS(opts *EvalOptions) *RaceContext {
	rc.branches = append(rc.branches, &raceBranch{
		condition: func(p *Page) (*Element, error) { return p.ElementByJS(opts) },
	})
	return rc
}

// Handle adds a callback function to the most recent chained selector.
// Handle 为最近的链式选择器添加一个回调函数。
// The callback function is run, if the corresponding selector is
// present first, in the Race condition.
// 如果相应的选择器首先出现在 trace 条件中，回调函数就会运行。
func (rc *RaceContext) Handle(callback func(*Element) error) *RaceContext {
	rc.branches[len(rc.branches)-1].callback = callback
	return rc
}

// Do the race
// 执行 Trace
func (rc *RaceContext) Do() (*Element, error) {
	var el *Element
	err := utils.Retry(rc.page.ctx, rc.page.sleeper(), func() (stop bool, err error) {
		for _, branch := range rc.branches {
			bEl, err := branch.condition(rc.page.Sleeper(NotFoundSleeper))
			if err == nil {
				el = bEl.Sleeper(rc.page.sleeper)

				if branch.callback != nil {
					err = branch.callback(el)
				}
				return true, err
			} else if !errors.Is(err, &ErrElementNotFound{}) {
				return true, err
			}
		}
		return
	})
	return el, err
}

// Has an element that matches the css selector
// 判断是否有和CSS选择器相匹配的元素
func (el *Element) Has(selector string) (bool, *Element, error) {
	el, err := el.Element(selector)
	if errors.Is(err, &ErrElementNotFound{}) {
		return false, nil, nil
	}
	return err == nil, el, err
}

// HasX an element that matches the XPath selector
// 判断是否有和XPath相匹配的元素
func (el *Element) HasX(selector string) (bool, *Element, error) {
	el, err := el.ElementX(selector)
	if errors.Is(err, &ErrElementNotFound{}) {
		return false, nil, nil
	}
	return err == nil, el, err
}

// HasR returns true if a child element that matches the css selector and its text matches the jsRegex.
// 如果有一个符合css选择器的子元素，并且其文本符合jsRegex，则HasR返回true。
func (el *Element) HasR(selector, jsRegex string) (bool, *Element, error) {
	el, err := el.ElementR(selector, jsRegex)
	if errors.Is(err, &ErrElementNotFound{}) {
		return false, nil, nil
	}
	return err == nil, el, err
}

// Element returns the first child that matches the css selector
// 返回第一个和CSS选择器匹配的子元素
func (el *Element) Element(selector string) (*Element, error) {
	return el.ElementByJS(evalHelper(js.Element, selector))
}

// ElementR returns the first child element that matches the css selector and its text matches the jsRegex.
// ElementR返回符合css选择器的第一个子元素，并且其文本符合jsRegex。
func (el *Element) ElementR(selector, jsRegex string) (*Element, error) {
	return el.ElementByJS(evalHelper(js.ElementR, selector, jsRegex))
}

// ElementX returns the first child that matches the XPath selector
// 返回第一个和 XPath 选择器相匹配的子元素
func (el *Element) ElementX(xPath string) (*Element, error) {
	return el.ElementByJS(evalHelper(js.ElementX, xPath))
}

// ElementByJS returns the element from the return value of the js
// ElementByJS 从 js 的返回值中返回该元素。
func (el *Element) ElementByJS(opts *EvalOptions) (*Element, error) {
	e, err := el.page.Sleeper(NotFoundSleeper).ElementByJS(opts.This(el.Object))
	if err != nil {
		return nil, err
	}
	return e.Sleeper(el.sleeper), nil
}

// Parent returns the parent element in the DOM tree
// 返回 DOM 树中的父元素。
func (el *Element) Parent() (*Element, error) {
	return el.ElementByJS(Eval(`() => this.parentElement`))
}

// Parents that match the selector
// 和选择器匹配的父元素。
func (el *Element) Parents(selector string) (Elements, error) {
	return el.ElementsByJS(evalHelper(js.Parents, selector))
}

// Next returns the next sibling element in the DOM tree
// 返回DOM树中的下一个同级元素
func (el *Element) Next() (*Element, error) {
	return el.ElementByJS(Eval(`() => this.nextElementSibling`))
}

// Previous returns the previous sibling element in the DOM tree
// 返回DOM树中的上一个同级元素
func (el *Element) Previous() (*Element, error) {
	return el.ElementByJS(Eval(`() => this.previousElementSibling`))
}

// Elements returns all elements that match the css selector
// 返回和 CSS 选择器相匹配的所有元素
func (el *Element) Elements(selector string) (Elements, error) {
	return el.ElementsByJS(evalHelper(js.Elements, selector))
}

// ElementsX returns all elements that match the XPath selector
// 返回和 XPath 选择器相匹配的所有元素
func (el *Element) ElementsX(xpath string) (Elements, error) {
	return el.ElementsByJS(evalHelper(js.ElementsX, xpath))
}

// ElementsByJS returns the elements from the return value of the js
// ElementsByJS 从 js 的返回值中返回元素。
func (el *Element) ElementsByJS(opts *EvalOptions) (Elements, error) {
	return el.page.Context(el.ctx).ElementsByJS(opts.This(el.Object))
}
