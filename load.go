package bulletml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
)

type bulletmlError struct {
	text string
	node node
}

func newBulletmlError(text string, node node) *bulletmlError {
	return &bulletmlError{
		text: text,
		node: node,
	}
}

func (e *bulletmlError) Error() string {
	buf := fmt.Sprintf("<%s>", e.node.xmlName())
	n := e.node.parent()
	for n != nil {
		buf = fmt.Sprintf("<%s> => ", n.xmlName()) + buf
		n = n.parent()
	}

	return fmt.Sprintf("%s (in %s)", e.text, buf)
}

// Load loads data from src and returns BulletML object.
func Load(src io.Reader) (*BulletML, error) {
	var b BulletML
	if err := xml.NewDecoder(src).Decode(&b); err != nil {
		return nil, err
	}

	return &b, nil
}

func prepareNodeTree(b *BulletML) error {
	return b.prepare()
}

func isIn[T comparable](v T, target []T) bool {
	for _, t := range target {
		if v == t {
			return true
		}
	}
	return false
}

type BulletMLType string

const (
	BulletMLTypeNone       BulletMLType = "none"
	BulletMLTypeVertical   BulletMLType = "vertical"
	BulletMLTypeHorizontal BulletMLType = "horizontal"
)

type BulletML struct {
	XMLName xml.Name     `xml:"bulletml"`
	Type    BulletMLType `xml:"type,attr"`
	Bullets []*Bullet    `xml:"bullet"`
	Actions []*Action    `xml:"action"`
	Fires   []*Fire      `xml:"fire"`
	Comment string       `xml:",comment"`
}

func (b *BulletML) prepare() error {
	if b.Type == "" {
		b.Type = BulletMLTypeNone
	}
	if !isIn(b.Type, []BulletMLType{BulletMLTypeNone, BulletMLTypeVertical, BulletMLTypeHorizontal}) {
		return newBulletmlError(fmt.Sprintf("Invalid 'type' attribute value of <%s> element: %s", b.XMLName.Local, b.Type), b)
	}

	for i := 0; i < len(b.Bullets); i++ {
		b.Bullets[i].parentNode = b
		if err := b.Bullets[i].prepare(); err != nil {
			return err
		}
	}

	for i := 0; i < len(b.Actions); i++ {
		b.Actions[i].parentNode = b
		if err := b.Actions[i].prepare(); err != nil {
			return err
		}
	}

	for i := 0; i < len(b.Fires); i++ {
		b.Fires[i].parentNode = b
		if err := b.Fires[i].prepare(); err != nil {
			return err
		}
	}

	return nil
}

func (b *BulletML) parent() node {
	return nil
}

func (b *BulletML) xmlName() string {
	return b.XMLName.Local
}

type Bullet struct {
	XMLName      xml.Name           `xml:"bullet"`
	Label        string             `xml:"label,attr,omitempty"`
	Direction    *Option[Direction] `xml:"direction,omitempty"`
	Speed        *Option[Speed]     `xml:"speed,omitempty"`
	ActionOrRefs []any              `xml:",any"`
	Comment      string             `xml:",comment"`
	parentNode   node               `xml:"-"`
}

func (b *Bullet) prepare() error {
	if d, exists := b.Direction.Get(); exists {
		d.parentNode = b
		if err := d.prepare(); err != nil {
			return err
		}
	}

	if s, exists := b.Speed.Get(); exists {
		s.parentNode = b
		if err := s.prepare(); err != nil {
			return err
		}
	}

	for i := 0; i < len(b.ActionOrRefs); i++ {
		switch a := b.ActionOrRefs[i].(type) {
		case *Action:
			a.parentNode = b
			if err := a.prepare(); err != nil {
				return err
			}
		case *ActionRef:
			a.parentNode = b
			if err := a.prepare(); err != nil {
				return err
			}
		default:
			return newBulletmlError(fmt.Sprintf("Invalid child element of <%s>: %T", b.XMLName.Local, a), b)
		}
	}

	return nil
}

func (b *Bullet) parent() node {
	return b.parentNode
}

func (b *Bullet) xmlName() string {
	return b.XMLName.Local
}

func (b *Bullet) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	b.XMLName = start.Name

	for _, attr := range start.Attr {
		if attr.Name.Local == "label" {
			b.Label = attr.Value
		}
	}

	b.Direction = &Option[Direction]{value: nil}
	b.Speed = &Option[Speed]{value: nil}

	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if c, ok := token.(xml.Comment); ok {
			b.Comment += string(c)
		} else if s, ok := token.(xml.StartElement); ok {
			switch s.Name.Local {
			case "direction":
				var dir Direction
				if err := d.DecodeElement(&dir, &s); err != nil {
					return err
				}
				b.Direction = &Option[Direction]{value: &dir}
			case "speed":
				var spd Speed
				if err := d.DecodeElement(&spd, &s); err != nil {
					return err
				}
				b.Speed = &Option[Speed]{value: &spd}
			case "action":
				var a Action
				if err := d.DecodeElement(&a, &s); err != nil {
					return err
				}
				b.ActionOrRefs = append(b.ActionOrRefs, &a)
			case "actionRef":
				var a ActionRef
				if err := d.DecodeElement(&a, &s); err != nil {
					return err
				}
				b.ActionOrRefs = append(b.ActionOrRefs, &a)
			default:
				return fmt.Errorf("Unexpected element <%s> in <bullet>", s.Name.Local)
			}
		}
	}

	return nil
}

type Action struct {
	XMLName    xml.Name `xml:"action"`
	Label      string   `xml:"label,attr,omitempty"`
	Commands   []any    `xml:",any"`
	Comment    string   `xml:",comment"`
	parentNode node     `xml:"-"`
}

func (a *Action) prepare() error {
	for i := 0; i < len(a.Commands); i++ {
		switch c := a.Commands[i].(type) {
		case *Repeat:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
		case *Fire:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
		case *FireRef:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
		case *ChangeSpeed:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
		case *ChangeDirection:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
		case *Accel:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
		case *Wait:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
		case *Vanish:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
		case *Action:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
		case *ActionRef:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
		default:
			return newBulletmlError(fmt.Sprintf("Invalid child element of <%s>: %T", a.XMLName.Local, c), a)
		}
	}

	return nil
}

func (a *Action) parent() node {
	return a.parentNode
}

func (a *Action) xmlName() string {
	return a.XMLName.Local
}

func (a *Action) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	a.XMLName = start.Name

	for _, attr := range start.Attr {
		if attr.Name.Local == "label" {
			a.Label = attr.Value
		}
	}

	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if c, ok := token.(xml.Comment); ok {
			a.Comment += string(c)
		} else if s, ok := token.(xml.StartElement); ok {
			switch s.Name.Local {
			case "repeat":
				var r Repeat
				if err := d.DecodeElement(&r, &s); err != nil {
					return err
				}
				a.Commands = append(a.Commands, &r)
			case "fire":
				var f Fire
				if err := d.DecodeElement(&f, &s); err != nil {
					return err
				}
				a.Commands = append(a.Commands, &f)
			case "fireRef":
				var f FireRef
				if err := d.DecodeElement(&f, &s); err != nil {
					return err
				}
				a.Commands = append(a.Commands, &f)
			case "changeSpeed":
				var c ChangeSpeed
				if err := d.DecodeElement(&c, &s); err != nil {
					return err
				}
				a.Commands = append(a.Commands, &c)
			case "changeDirection":
				var c ChangeDirection
				if err := d.DecodeElement(&c, &s); err != nil {
					return err
				}
				a.Commands = append(a.Commands, &c)
			case "accel":
				var ac Accel
				if err := d.DecodeElement(&ac, &s); err != nil {
					return err
				}
				a.Commands = append(a.Commands, &ac)
			case "wait":
				var w Wait
				if err := d.DecodeElement(&w, &s); err != nil {
					return err
				}
				a.Commands = append(a.Commands, &w)
			case "vanish":
				var v Vanish
				if err := d.DecodeElement(&v, &s); err != nil {
					return err
				}
				a.Commands = append(a.Commands, &v)
			case "action":
				var ac Action
				if err := d.DecodeElement(&ac, &s); err != nil {
					return err
				}
				a.Commands = append(a.Commands, &ac)
			case "actionRef":
				var ac ActionRef
				if err := d.DecodeElement(&ac, &s); err != nil {
					return err
				}
				a.Commands = append(a.Commands, &ac)
			default:
				return fmt.Errorf("Unexpected element <%s> in <action>", s.Name.Local)
			}
		}
	}

	return nil
}

type Fire struct {
	XMLName    xml.Name           `xml:"fire"`
	Label      string             `xml:"label,attr,omitempty"`
	Direction  *Option[Direction] `xml:"direction,omitempty"`
	Speed      *Option[Speed]     `xml:"speed,omitempty"`
	Bullet     *Option[Bullet]    `xml:"bullet,omitempty"`
	BulletRef  *Option[BulletRef] `xml:"bulletRef,omitempty"`
	Comment    string             `xml:",comment"`
	parentNode node               `xml:"-"`
}

func (f *Fire) prepare() error {
	if d, exists := f.Direction.Get(); exists {
		d.parentNode = f
		if err := d.prepare(); err != nil {
			return err
		}
	}

	if s, exists := f.Speed.Get(); exists {
		s.parentNode = f
		if err := s.prepare(); err != nil {
			return err
		}
	}

	b, bulletExists := f.Bullet.Get()
	br, bulletRefExists := f.BulletRef.Get()

	if bulletExists && bulletRefExists {
		return newBulletmlError(fmt.Sprintf("Both <%s> and <%s> exist in <%s> element", b.XMLName.Local, br.XMLName.Local, f.XMLName.Local), f)
	}
	if !bulletExists && !bulletRefExists {
		return newBulletmlError(fmt.Sprintf("Either <%s> or <%s> required in <%s> element", getFieldXmlName(f, "Bullet"), getFieldXmlName(f, "BulletRef"), f.XMLName.Local), f)
	}

	if bulletExists {
		b.parentNode = f
		if err := b.prepare(); err != nil {
			return err
		}
	}

	if bulletRefExists {
		br.parentNode = f
		if err := br.prepare(); err != nil {
			return err
		}
	}

	return nil
}

func (f *Fire) parent() node {
	return f.parentNode
}

func (f *Fire) xmlName() string {
	return f.XMLName.Local
}

func (f *Fire) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type F Fire

	var fr F
	if err := d.DecodeElement(&fr, &start); err != nil {
		return err
	}

	*f = Fire(fr)

	if f.Direction == nil {
		f.Direction = &Option[Direction]{value: nil}
	}
	if f.Speed == nil {
		f.Speed = &Option[Speed]{value: nil}
	}
	if f.Bullet == nil {
		f.Bullet = &Option[Bullet]{value: nil}
	}
	if f.BulletRef == nil {
		f.BulletRef = &Option[BulletRef]{value: nil}
	}

	return nil
}

type ChangeDirection struct {
	XMLName    xml.Name   `xml:"changeDirection"`
	Direction  *Direction `xml:"direction"`
	Term       *Term      `xml:"term"`
	Comment    string     `xml:",comment"`
	parentNode node       `xml:"-"`
}

func (c *ChangeDirection) prepare() error {
	if c.Direction == nil {
		return newBulletmlError(fmt.Sprintf("<%s> required in <%s>", getFieldXmlName(c, "Direction"), c.XMLName.Local), c)
	}
	c.Direction.parentNode = c
	if err := c.Direction.prepare(); err != nil {
		return err
	}

	if c.Term == nil {
		return newBulletmlError(fmt.Sprintf("<%s> required in <%s>", getFieldXmlName(c, "Term"), c.XMLName.Local), c)
	}
	c.Term.parentNode = c
	if err := c.Term.prepare(); err != nil {
		return err
	}

	return nil
}

func (c *ChangeDirection) parent() node {
	return c.parentNode
}

func (c *ChangeDirection) xmlName() string {
	return c.XMLName.Local
}

type ChangeSpeed struct {
	XMLName    xml.Name `xml:"changeSpeed"`
	Speed      *Speed   `xml:"speed"`
	Term       *Term    `xml:"term"`
	Comment    string   `xml:",comment"`
	parentNode node     `xml:"-"`
}

func (c *ChangeSpeed) prepare() error {
	if c.Speed == nil {
		return newBulletmlError(fmt.Sprintf("<%s> required in <%s>", getFieldXmlName(c, "Speed"), c.XMLName.Local), c)
	}
	c.Speed.parentNode = c
	if err := c.Speed.prepare(); err != nil {
		return err
	}

	if c.Term == nil {
		return newBulletmlError(fmt.Sprintf("<%s> required in <%s>", getFieldXmlName(c, "Term"), c.XMLName.Local), c)
	}
	c.Term.parentNode = c
	if err := c.Term.prepare(); err != nil {
		return err
	}

	return nil
}

func (c *ChangeSpeed) parent() node {
	return c.parentNode
}

func (c *ChangeSpeed) xmlName() string {
	return c.XMLName.Local
}

type Accel struct {
	XMLName    xml.Name            `xml:"accel"`
	Horizontal *Option[Horizontal] `xml:"horizontal,omitempty"`
	Vertical   *Option[Vertical]   `xml:"vertical,omitempty"`
	Term       *Term               `xml:"term"`
	Comment    string              `xml:",comment"`
	parentNode node                `xml:"-"`
}

func (a *Accel) prepare() error {
	if h, exists := a.Horizontal.Get(); exists {
		h.parentNode = a
		if err := h.prepare(); err != nil {
			return err
		}
	}

	if v, exists := a.Vertical.Get(); exists {
		v.parentNode = a
		if err := v.prepare(); err != nil {
			return err
		}
	}

	if a.Term == nil {
		return newBulletmlError(fmt.Sprintf("<%s> required in <%s>", getFieldXmlName(a, "Term"), a.XMLName.Local), a)
	}
	a.Term.parentNode = a
	if err := a.Term.prepare(); err != nil {
		return err
	}

	return nil
}

func (a *Accel) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type A Accel

	var ac A
	if err := d.DecodeElement(&ac, &start); err != nil {
		return err
	}

	*a = Accel(ac)

	if a.Horizontal == nil {
		a.Horizontal = &Option[Horizontal]{value: nil}
	}
	if a.Vertical == nil {
		a.Vertical = &Option[Vertical]{value: nil}
	}

	return nil
}

func (a *Accel) parent() node {
	return a.parentNode
}

func (a *Accel) xmlName() string {
	return a.XMLName.Local
}

type Wait struct {
	XMLName      xml.Name `xml:"wait"`
	Expr         string   `xml:",chardata"`
	Comment      string   `xml:",comment"`
	compiledExpr ast.Expr `xml:"-"`
	parentNode   node     `xml:"-"`
}

func (w *Wait) prepare() error {
	compiled, err := compileExpr(w.Expr, w)
	if err != nil {
		return err
	}
	w.compiledExpr = compiled

	return nil
}

func (w *Wait) parent() node {
	return w.parentNode
}

func (w *Wait) xmlName() string {
	return w.XMLName.Local
}

type Vanish struct {
	XMLName    xml.Name `xml:"vanish"`
	Comment    string   `xml:",comment"`
	parentNode node     `xml:"-"`
}

func (v *Vanish) prepare() error {
	return nil
}

func (v *Vanish) parent() node {
	return v.parentNode
}

func (v *Vanish) xmlName() string {
	return v.XMLName.Local
}

type Repeat struct {
	XMLName    xml.Name           `xml:"repeat"`
	Times      *Times             `xml:"times"`
	Action     *Option[Action]    `xml:"action,omitempty"`
	ActionRef  *Option[ActionRef] `xml:"actionRef,omitempty"`
	Comment    string             `xml:",comment"`
	parentNode node               `xml:"-"`
}

func (r *Repeat) prepare() error {
	if r.Times == nil {
		return newBulletmlError(fmt.Sprintf("<%s> required in <%s>", getFieldXmlName(r, "Times"), r.XMLName.Local), r)
	}
	r.Times.parentNode = r
	if err := r.Times.prepare(); err != nil {
		return err
	}

	a, actionExists := r.Action.Get()
	ar, actionRefExists := r.ActionRef.Get()

	if actionExists && actionRefExists {
		return newBulletmlError(fmt.Sprintf("Both <%s> and <%s> exist in <%s> element", a.XMLName.Local, ar.XMLName.Local, r.XMLName.Local), r)
	}
	if !actionExists && !actionRefExists {
		return newBulletmlError(fmt.Sprintf("Either <%s> or <%s> required in <%s> element", getFieldXmlName(r, "Action"), getFieldXmlName(r, "ActionRef"), r.XMLName.Local), r)
	}

	if actionExists {
		a.parentNode = r
		if err := a.prepare(); err != nil {
			return err
		}
	}

	if actionRefExists {
		ar.parentNode = r
		if err := ar.prepare(); err != nil {
			return err
		}
	}

	return nil
}

func (r *Repeat) parent() node {
	return r.parentNode
}

func (r *Repeat) xmlName() string {
	return r.XMLName.Local
}

func (r *Repeat) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type R Repeat

	var rp R
	if err := d.DecodeElement(&rp, &start); err != nil {
		return err
	}

	*r = Repeat(rp)

	if r.Action == nil {
		r.Action = &Option[Action]{value: nil}
	}
	if r.ActionRef == nil {
		r.ActionRef = &Option[ActionRef]{value: nil}
	}

	return nil
}

type DirectionType string

const (
	DirectionTypeAim      DirectionType = "aim"
	DirectionTypeAbsolute DirectionType = "absolute"
	DirectionTypeRelative DirectionType = "relative"
	DirectionTypeSequence DirectionType = "sequence"
)

type Direction struct {
	XMLName      xml.Name      `xml:"direction"`
	Type         DirectionType `xml:"type,attr"`
	Expr         string        `xml:",chardata"`
	Comment      string        `xml:",comment"`
	compiledExpr ast.Expr      `xml:"-"`
	parentNode   node          `xml:"-"`
}

func (d *Direction) prepare() error {
	if d.Type == "" {
		d.Type = DirectionTypeAim
	}
	if !isIn(d.Type, []DirectionType{DirectionTypeAim, DirectionTypeAbsolute, DirectionTypeRelative, DirectionTypeSequence}) {
		return newBulletmlError(fmt.Sprintf("Invalid 'type' attribute value of <%s> element: %s", d.XMLName.Local, d.Type), d)
	}

	compiled, err := compileExpr(d.Expr, d)
	if err != nil {
		return err
	}
	d.compiledExpr = compiled

	return nil
}

func (d *Direction) parent() node {
	return d.parentNode
}

func (d *Direction) xmlName() string {
	return d.XMLName.Local
}

type SpeedType string

const (
	SpeedTypeAbsolute SpeedType = "absolute"
	SpeedTypeRelative SpeedType = "relative"
	SpeedTypeSequence SpeedType = "sequence"
)

type Speed struct {
	XMLName      xml.Name  `xml:"speed"`
	Type         SpeedType `xml:"type,attr"`
	Expr         string    `xml:",chardata"`
	Comment      string    `xml:",comment"`
	compiledExpr ast.Expr  `xml:"-"`
	parentNode   node      `xml:"-"`
}

func (s *Speed) prepare() error {
	if s.Type == "" {
		s.Type = SpeedTypeAbsolute
	}
	if !isIn(s.Type, []SpeedType{SpeedTypeAbsolute, SpeedTypeRelative, SpeedTypeSequence}) {
		return newBulletmlError(fmt.Sprintf("Invalid 'type' attribute value of <%s> element: %s", s.XMLName.Local, s.Type), s)
	}

	compiled, err := compileExpr(s.Expr, s)
	if err != nil {
		return err
	}
	s.compiledExpr = compiled

	return nil
}

func (s *Speed) parent() node {
	return s.parentNode
}

func (s *Speed) xmlName() string {
	return s.XMLName.Local
}

type HorizontalType string

const (
	HorizontalTypeAbsolute HorizontalType = "absolute"
	HorizontalTypeRelative HorizontalType = "relative"
	HorizontalTypeSequence HorizontalType = "sequence"
)

type Horizontal struct {
	XMLName      xml.Name       `xml:"horizontal"`
	Type         HorizontalType `xml:"type,attr"`
	Expr         string         `xml:",chardata"`
	Comment      string         `xml:",comment"`
	compiledExpr ast.Expr       `xml:"-"`
	parentNode   node           `xml:"-"`
}

func (h *Horizontal) prepare() error {
	if h.Type == "" {
		h.Type = HorizontalTypeAbsolute
	}
	if !isIn(h.Type, []HorizontalType{HorizontalTypeAbsolute, HorizontalTypeRelative, HorizontalTypeSequence}) {
		return newBulletmlError(fmt.Sprintf("Invalid 'type' attribute value of <%s> element: %s", h.XMLName.Local, h.Type), h)
	}

	compiled, err := compileExpr(h.Expr, h)
	if err != nil {
		return err
	}
	h.compiledExpr = compiled

	return nil
}

func (h *Horizontal) parent() node {
	return h.parentNode
}

func (h *Horizontal) xmlName() string {
	return h.XMLName.Local
}

type VerticalType string

const (
	VerticalTypeAbsolute VerticalType = "absolute"
	VerticalTypeRelative VerticalType = "relative"
	VerticalTypeSequence VerticalType = "sequence"
)

type Vertical struct {
	XMLName      xml.Name     `xml:"vertical"`
	Type         VerticalType `xml:"type,attr"`
	Expr         string       `xml:",chardata"`
	Comment      string       `xml:",comment"`
	compiledExpr ast.Expr     `xml:"-"`
	parentNode   node         `xml:"-"`
}

func (v *Vertical) prepare() error {
	if v.Type == "" {
		v.Type = VerticalTypeAbsolute
	}
	if !isIn(v.Type, []VerticalType{VerticalTypeAbsolute, VerticalTypeRelative, VerticalTypeSequence}) {
		return newBulletmlError(fmt.Sprintf("Invalid 'type' attribute value of <%s> element: %s", v.XMLName.Local, v.Type), v)
	}

	compiled, err := compileExpr(v.Expr, v)
	if err != nil {
		return err
	}
	v.compiledExpr = compiled

	return nil
}

func (v *Vertical) parent() node {
	return v.parentNode
}

func (v *Vertical) xmlName() string {
	return v.XMLName.Local
}

type Term struct {
	XMLName      xml.Name `xml:"term"`
	Expr         string   `xml:",chardata"`
	Comment      string   `xml:",comment"`
	compiledExpr ast.Expr `xml:"-"`
	parentNode   node     `xml:"-"`
}

func (t *Term) prepare() error {
	compiled, err := compileExpr(t.Expr, t)
	if err != nil {
		return err
	}
	t.compiledExpr = compiled

	return nil
}

func (t *Term) parent() node {
	return t.parentNode
}

func (t *Term) xmlName() string {
	return t.XMLName.Local
}

type Times struct {
	XMLName      xml.Name `xml:"times"`
	Expr         string   `xml:",chardata"`
	Comment      string   `xml:",comment"`
	compiledExpr ast.Expr `xml:"-"`
	parentNode   node     `xml:"-"`
}

func (t *Times) prepare() error {
	compiled, err := compileExpr(t.Expr, t)
	if err != nil {
		return err
	}
	t.compiledExpr = compiled

	return nil
}

func (t *Times) parent() node {
	return t.parentNode
}

func (t *Times) xmlName() string {
	return t.XMLName.Local
}

type BulletRef struct {
	XMLName    xml.Name `xml:"bulletRef"`
	Label      string   `xml:"label,attr"`
	Params     []*Param `xml:"param"`
	Comment    string   `xml:",comment"`
	parentNode node     `xml:"-"`
}

func (b *BulletRef) prepare() error {
	if b.Label == "" {
		return newBulletmlError(fmt.Sprintf("<%s> element requires 'label' attribute", b.XMLName.Local), b)
	}

	for i := 0; i < len(b.Params); i++ {
		b.Params[i].parentNode = b
		if err := b.Params[i].prepare(); err != nil {
			return err
		}
	}

	return nil
}

func (b *BulletRef) parent() node {
	return b.parentNode
}

func (b *BulletRef) xmlName() string {
	return b.XMLName.Local
}

func (b *BulletRef) label() string {
	return b.Label
}

func (b *BulletRef) params() []*Param {
	return b.Params
}

type ActionRef struct {
	XMLName    xml.Name `xml:"actionRef"`
	Label      string   `xml:"label,attr"`
	Params     []*Param `xml:"param"`
	Comment    string   `xml:",comment"`
	parentNode node     `xml:"-"`
}

func (a *ActionRef) prepare() error {
	if a.Label == "" {
		return newBulletmlError(fmt.Sprintf("<%s> element requires 'label' attribute", a.XMLName.Local), a)
	}

	for i := 0; i < len(a.Params); i++ {
		a.Params[i].parentNode = a
		if err := a.Params[i].prepare(); err != nil {
			return err
		}
	}

	return nil
}

func (a *ActionRef) parent() node {
	return a.parentNode
}

func (a *ActionRef) xmlName() string {
	return a.XMLName.Local
}

func (a *ActionRef) label() string {
	return a.Label
}

func (a *ActionRef) params() []*Param {
	return a.Params
}

type FireRef struct {
	XMLName    xml.Name `xml:"fireRef"`
	Label      string   `xml:"label,attr"`
	Params     []*Param `xml:"param"`
	Comment    string   `xml:",comment"`
	parentNode node     `xml:"-"`
}

func (f *FireRef) prepare() error {
	if f.Label == "" {
		return newBulletmlError(fmt.Sprintf("<%s> element requires 'label' attribute", f.XMLName.Local), f)
	}

	for i := 0; i < len(f.Params); i++ {
		f.Params[i].parentNode = f
		if err := f.Params[i].prepare(); err != nil {
			return err
		}
	}

	return nil
}

func (f *FireRef) parent() node {
	return f.parentNode
}

func (f *FireRef) xmlName() string {
	return f.XMLName.Local
}

func (f *FireRef) label() string {
	return f.Label
}

func (f *FireRef) params() []*Param {
	return f.Params
}

type Param struct {
	XMLName      xml.Name `xml:"param"`
	Expr         string   `xml:",chardata"`
	Comment      string   `xml:",comment"`
	compiledExpr ast.Expr `xml:"-"`
	parentNode   node     `xml:"-"`
}

func (p *Param) prepare() error {
	compiled, err := compileExpr(p.Expr, p)
	if err != nil {
		return err
	}
	p.compiledExpr = compiled

	return nil
}

func (p *Param) parent() node {
	return p.parentNode
}

func (p *Param) xmlName() string {
	return p.XMLName.Local
}

type node interface {
	xmlName() string
	parent() node
}

type refType interface {
	node
	label() string
	params() []*Param
}

func getFieldXmlName(ptr any, fieldName string) string {
	t := reflect.TypeOf(ptr).Elem()

	f, ok := t.FieldByName(fieldName)
	if !ok {
		panic(fmt.Sprintf("%s has no field '%s'", t.Name(), fieldName))
	}
	return strings.Split(f.Tag.Get("xml"), ",")[0]
}

type Option[T any] struct {
	value *T
}

func (o *Option[T]) Get() (*T, bool) {
	if o.value != nil {
		return o.value, true
	} else {
		return nil, false
	}
}

func (o *Option[T]) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var v T
	if err := d.DecodeElement(&v, &start); err != nil {
		return err
	}

	o.value = &v

	return nil
}

func compileExpr(expr string, node node) (ast.Expr, error) {
	expr = strings.ReplaceAll(expr, "$", "V_")
	expr = strings.ReplaceAll(expr, "V_loop.", "V_loop_")

	root, err := parser.ParseExpr(expr)
	if err != nil {
		return nil, newBulletmlError(err.Error(), node)
	}

	return compileAst(root, node)
}

type numberValue struct {
	ast.Expr
	value float64
}

func compileAst(node ast.Expr, bmlNode node) (ast.Expr, error) {
	switch e := node.(type) {
	case *ast.BinaryExpr:
		x, err := compileAst(e.X, bmlNode)
		if err != nil {
			return nil, err
		}
		y, err := compileAst(e.Y, bmlNode)
		if err != nil {
			return nil, err
		}
		xv, xok := x.(*numberValue)
		yv, yok := y.(*numberValue)
		if xok && yok {
			switch e.Op {
			case token.ADD:
				return &numberValue{value: xv.value + yv.value}, nil
			case token.SUB:
				return &numberValue{value: xv.value - yv.value}, nil
			case token.MUL:
				return &numberValue{value: xv.value * yv.value}, nil
			case token.QUO:
				return &numberValue{value: xv.value / yv.value}, nil
			case token.REM:
				return &numberValue{value: float64(int64(xv.value) % int64(yv.value))}, nil
			default:
				return nil, newBulletmlError(fmt.Sprintf("Unsupported operator: %s", e.Op.String()), bmlNode)
			}
		}

		if xok {
			e.X = xv
		}
		if yok {
			e.Y = yv
		}

		return e, nil
	case *ast.UnaryExpr:
		x, err := compileAst(e.X, bmlNode)
		if err != nil {
			return nil, err
		}
		if xv, ok := x.(*numberValue); ok {
			switch e.Op {
			case token.SUB:
				return &numberValue{value: -xv.value}, nil
			default:
				return nil, newBulletmlError(fmt.Sprintf("Unsupported operator: %s", e.Op.String()), bmlNode)
			}
		} else {
			return e, nil
		}
	case *ast.BasicLit:
		switch e.Kind {
		case token.FLOAT, token.INT:
			v, err := strconv.ParseFloat(e.Value, 64)
			if err != nil {
				return nil, newBulletmlError(fmt.Sprintf("Invalid number value (%s): %s", err.Error(), e.Value), bmlNode)
			}
			return &numberValue{value: v}, nil
		default:
			return nil, newBulletmlError(fmt.Sprintf("Unsupported literal: %s", e.Value), bmlNode)
		}
	case *ast.Ident:
		name := e.Name
		name = strings.ReplaceAll(name, "V_loop_", "V_loop.")
		name = strings.ReplaceAll(name, "V_", "$")
		e.Name = name
		return e, nil
	case *ast.CallExpr:
		f, ok := e.Fun.(*ast.Ident)
		if !ok {
			var buf bytes.Buffer
			if err := format.Node(&buf, token.NewFileSet(), e.Fun); err != nil {
				return nil, newBulletmlError(err.Error(), bmlNode)
			}
			return nil, newBulletmlError(fmt.Sprintf("Unsupported function: %s", string(buf.Bytes())), bmlNode)
		}

		var args []float64
		for i, arg := range e.Args {
			a, err := compileAst(arg, bmlNode)
			if err != nil {
				return nil, err
			}
			e.Args[i] = a
			if v, ok := a.(*numberValue); ok {
				args = append(args, v.value)
			}
		}
		if len(args) != len(e.Args) {
			return e, nil
		}

		switch f.Name {
		case "sin":
			if len(args) < 1 {
				return nil, newBulletmlError(fmt.Sprintf("Too few arguments for sin(): %d", len(args)), bmlNode)
			}
			arg := args[0] * math.Pi / 180
			return &numberValue{value: math.Sin(arg)}, nil
		case "cos":
			if len(args) < 1 {
				return nil, newBulletmlError(fmt.Sprintf("Too few arguments for cos(): %d", len(args)), bmlNode)
			}
			arg := args[0] * math.Pi / 180
			return &numberValue{value: math.Cos(arg)}, nil
		default:
			return e, nil
		}
	case *ast.ParenExpr:
		return compileAst(e.X, bmlNode)
	default:
		var buf bytes.Buffer
		if err := format.Node(&buf, token.NewFileSet(), node); err != nil {
			return nil, err
		}
		return nil, newBulletmlError(fmt.Sprintf("Unsupported expression: %s", string(buf.Bytes())), bmlNode)
	}
}
