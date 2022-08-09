// Patches to normalize the proto types
// 用于规范化原型的补丁

package proto

import (
	"time"
)

// TimeSinceEpoch UTC time in seconds, counted from January 1, 1970.
// TimeSinceEpoch UTC时间，以秒为单位，从1970年1月1日算起。
// To convert a time.Time to TimeSinceEpoch, for example:
// 转换时间。例如：
//     proto.TimeSinceEpoch(time.Now().Unix())
// For session cookie, the value should be -1.
// 对于会话cookie，该值应该是-1。
type TimeSinceEpoch float64

// Time interface
func (t TimeSinceEpoch) Time() time.Time {
	return (time.Unix(0, 0)).Add(
		time.Duration(t * TimeSinceEpoch(time.Second)),
	)
}

// String interface
func (t TimeSinceEpoch) String() string {
	return t.Time().String()
}

// MonotonicTime Monotonically increasing time in seconds since an arbitrary point in the past.
// 单调时间（MonotonicTime） 从过去的一个任意点开始，以秒为单位单调地增加时间。
type MonotonicTime float64

// Duration interface
func (t MonotonicTime) Duration() time.Duration {
	return time.Duration(t * MonotonicTime(time.Second))
}

// String interface
func (t MonotonicTime) String() string {
	return t.Duration().String()
}

// Point from the origin (0, 0)
// 从原点（0，0）开始的点
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Len is the number of vertices
// Len是顶点的数量
func (q DOMQuad) Len() int {
	return len(q) / 2
}

// Each point
// 返回每一个点
func (q DOMQuad) Each(fn func(pt Point, i int)) {
	for i := 0; i < q.Len(); i++ {
		fn(Point{q[i*2], q[i*2+1]}, i)
	}
}

// Center of the polygon
// 多边形的中心
func (q DOMQuad) Center() Point {
	var x, y float64
	q.Each(func(pt Point, _ int) {
		x += pt.X
		y += pt.Y
	})
	return Point{x / float64(q.Len()), y / float64(q.Len())}
}

// Area of the polygon
// 多边形的面积
// https://en.wikipedia.org/wiki/Polygon#Area
func (q DOMQuad) Area() float64 {
	area := 0.0
	l := len(q)/2 - 1

	for i := 0; i < l; i++ {
		area += q[i*2]*q[i*2+3] - q[i*2+2]*q[i*2+1]
	}
	area += q[l*2]*q[1] - q[0]*q[l*2+1]

	return area / 2
}

// OnePointInside the shape
// 形状内部的一个点
func (res *DOMGetContentQuadsResult) OnePointInside() *Point {
	for _, q := range res.Quads {
		if q.Area() >= 1 {
			pt := q.Center()
			return &pt
		}
	}

	return nil
}

// Box returns the smallest leveled rectangle that can cover the whole shape.
// 返回可以覆盖整个形状的最小水平矩形。
func (res *DOMGetContentQuadsResult) Box() (box *DOMRect) {
	return Shape(res.Quads).Box()
}

// Shape is a list of DOMQuad
// Shape是DOMQuad的列表
type Shape []DOMQuad

// Box returns the smallest leveled rectangle that can cover the whole shape.
// 返回可以覆盖整个形状的最小水平矩形。
func (qs Shape) Box() (box *DOMRect) {
	if len(qs) == 0 {
		return
	}

	left := qs[0][0]
	top := qs[0][1]
	right := left
	bottom := top

	for _, q := range qs {
		q.Each(func(pt Point, _ int) {
			if pt.X < left {
				left = pt.X
			}
			if pt.Y < top {
				top = pt.Y
			}
			if pt.X > right {
				right = pt.X
			}
			if pt.Y > bottom {
				bottom = pt.Y
			}
		})
	}

	box = &DOMRect{left, top, right - left, bottom - top}

	return
}

// MoveTo X and Y to x and y
// 将X和Y移至x和y
func (p *InputTouchPoint) MoveTo(x, y float64) {
	p.X = x
	p.Y = y
}

// CookiesToParams converts Cookies list to NetworkCookieParam list
// 将Cookies列表转换为NetworkCookieParam列表
func CookiesToParams(cookies []*NetworkCookie) []*NetworkCookieParam {
	list := []*NetworkCookieParam{}
	for _, c := range cookies {
		list = append(list, &NetworkCookieParam{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
			SameSite: c.SameSite,
			Expires:  c.Expires,
			Priority: c.Priority,
		})
	}
	return list
}
