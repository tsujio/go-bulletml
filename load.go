package bulletml

import (
	"encoding/xml"
	"fmt"
	"io"
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

func Load(src io.Reader) (*BulletML, error) {
	var b BulletML
	if err := xml.NewDecoder(src).Decode(&b); err != nil {
		return nil, err
	}

	if err := prepareNodeTree(&b); err != nil {
		return nil, err
	}

	return &b, nil
}

func prepareNodeTree(b *BulletML) error {
	return b.prepare()
}

func decodeElement[T any](d *xml.Decoder, start *xml.StartElement) (T, error) {
	var v T
	if err := d.DecodeElement(&v, start); err != nil {
		return v, err
	}
	return v, nil
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
	Bullets []Bullet     `xml:"bullet"`
	Actions []Action     `xml:"action"`
	Fires   []Fire       `xml:"fire"`
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
	XMLName      xml.Name   `xml:"bullet"`
	Label        string     `xml:"label,attr,omitempty"`
	Direction    *Direction `xml:"direction,omitempty"`
	Speed        *Speed     `xml:"speed,omitempty"`
	ActionOrRefs []any      `xml:",any"`
	parentNode   node       `xml:"-"`
}

func (b *Bullet) prepare() error {
	if b.Direction != nil {
		b.Direction.parentNode = b
		if err := b.Direction.prepare(); err != nil {
			return err
		}
	}

	if b.Speed != nil {
		b.Speed.parentNode = b
		if err := b.Speed.prepare(); err != nil {
			return err
		}
	}

	for i := 0; i < len(b.ActionOrRefs); i++ {
		switch a := b.ActionOrRefs[i].(type) {
		case Action:
			a.parentNode = b
			if err := a.prepare(); err != nil {
				return err
			}
			b.ActionOrRefs[i] = a
		case ActionRef:
			a.parentNode = b
			if err := a.prepare(); err != nil {
				return err
			}
			b.ActionOrRefs[i] = a
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

	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if s, ok := token.(xml.StartElement); ok {
			switch s.Name.Local {
			case "direction":
				if d, err := decodeElement[Direction](d, &s); err != nil {
					return err
				} else {
					b.Direction = &d
				}
			case "speed":
				if sp, err := decodeElement[Speed](d, &s); err != nil {
					return err
				} else {
					b.Speed = &sp
				}
			case "action":
				if a, err := decodeElement[Action](d, &s); err != nil {
					return err
				} else {
					b.ActionOrRefs = append(b.ActionOrRefs, a)
				}
			case "actionRef":
				if a, err := decodeElement[ActionRef](d, &s); err != nil {
					return err
				} else {
					b.ActionOrRefs = append(b.ActionOrRefs, a)
				}
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
	parentNode node     `xml:"-"`
}

func (a *Action) prepare() error {
	for i := 0; i < len(a.Commands); i++ {
		switch c := a.Commands[i].(type) {
		case Repeat:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
			a.Commands[i] = c
		case Fire:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
			a.Commands[i] = c
		case FireRef:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
			a.Commands[i] = c
		case ChangeSpeed:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
			a.Commands[i] = c
		case ChangeDirection:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
			a.Commands[i] = c
		case Accel:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
			a.Commands[i] = c
		case Wait:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
			a.Commands[i] = c
		case Vanish:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
			a.Commands[i] = c
		case Action:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
			a.Commands[i] = c
		case ActionRef:
			c.parentNode = a
			if err := c.prepare(); err != nil {
				return err
			}
			a.Commands[i] = c
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
		if s, ok := token.(xml.StartElement); ok {
			var e any
			switch s.Name.Local {
			case "repeat":
				if e, err = decodeElement[Repeat](d, &s); err != nil {
					return err
				}
			case "fire":
				if e, err = decodeElement[Fire](d, &s); err != nil {
					return err
				}
			case "fireRef":
				if e, err = decodeElement[FireRef](d, &s); err != nil {
					return err
				}
			case "changeSpeed":
				if e, err = decodeElement[ChangeSpeed](d, &s); err != nil {
					return err
				}
			case "changeDirection":
				if e, err = decodeElement[ChangeDirection](d, &s); err != nil {
					return err
				}
			case "accel":
				if e, err = decodeElement[Accel](d, &s); err != nil {
					return err
				}
			case "wait":
				if e, err = decodeElement[Wait](d, &s); err != nil {
					return err
				}
			case "vanish":
				if e, err = decodeElement[Vanish](d, &s); err != nil {
					return err
				}
			case "action":
				if e, err = decodeElement[Action](d, &s); err != nil {
					return err
				}
			case "actionRef":
				if e, err = decodeElement[ActionRef](d, &s); err != nil {
					return err
				}
			default:
				return fmt.Errorf("Unexpected element <%s> in <action>", s.Name.Local)
			}

			a.Commands = append(a.Commands, e)
		}
	}

	return nil
}

type Fire struct {
	XMLName    xml.Name   `xml:"fire"`
	Label      string     `xml:"label,attr,omitempty"`
	Direction  *Direction `xml:"direction,omitempty"`
	Speed      *Speed     `xml:"speed,omitempty"`
	Bullet     *Bullet    `xml:"bullet,omitempty"`
	BulletRef  *BulletRef `xml:"bulletRef,omitempty"`
	parentNode node       `xml:"-"`
}

func (f *Fire) prepare() error {
	if f.Direction != nil {
		f.Direction.parentNode = f
		if err := f.Direction.prepare(); err != nil {
			return err
		}
	}

	if f.Speed != nil {
		f.Speed.parentNode = f
		if err := f.Speed.prepare(); err != nil {
			return err
		}
	}

	if f.Bullet != nil && f.BulletRef != nil {
		return newBulletmlError(fmt.Sprintf("Both <%s> and <%s> exist in <%s> element", f.Bullet.XMLName.Local, f.BulletRef.XMLName.Local, f.XMLName.Local), f)
	}

	if f.Bullet != nil {
		f.Bullet.parentNode = f
		if err := f.Bullet.prepare(); err != nil {
			return err
		}
	}

	if f.BulletRef != nil {
		f.BulletRef.parentNode = f
		if err := f.BulletRef.prepare(); err != nil {
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

type ChangeDirection struct {
	XMLName    xml.Name  `xml:"changeDirection"`
	Direction  Direction `xml:"direction"`
	Term       Term      `xml:"term"`
	parentNode node      `xml:"-"`
}

func (c *ChangeDirection) prepare() error {
	c.Direction.parentNode = c
	if err := c.Direction.prepare(); err != nil {
		return err
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
	Speed      Speed    `xml:"speed"`
	Term       Term     `xml:"term"`
	parentNode node     `xml:"-"`
}

func (c *ChangeSpeed) prepare() error {
	c.Speed.parentNode = c
	if err := c.Speed.prepare(); err != nil {
		return err
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
	XMLName    xml.Name    `xml:"accel"`
	Horizontal *Horizontal `xml:"horizontal,omitempty"`
	Vertical   *Vertical   `xml:"vertical,omitempty"`
	Term       Term        `xml:"term"`
	parentNode node        `xml:"-"`
}

func (a *Accel) prepare() error {
	if a.Horizontal != nil {
		a.Horizontal.parentNode = a
		if err := a.Horizontal.prepare(); err != nil {
			return err
		}
	}

	if a.Vertical != nil {
		a.Vertical.parentNode = a
		if err := a.Vertical.prepare(); err != nil {
			return err
		}
	}

	a.Term.parentNode = a
	if err := a.Term.prepare(); err != nil {
		return err
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
	XMLName    xml.Name `xml:"wait"`
	Expr       string   `xml:",chardata"`
	parentNode node     `xml:"-"`
}

func (w *Wait) prepare() error {
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
	XMLName    xml.Name   `xml:"repeat"`
	Times      Times      `xml:"times"`
	Action     *Action    `xml:"action,omitempty"`
	ActionRef  *ActionRef `xml:"actionRef,omitempty"`
	parentNode node       `xml:"-"`
}

func (r *Repeat) prepare() error {
	r.Times.parentNode = r
	if err := r.Times.prepare(); err != nil {
		return err
	}

	if r.Action != nil && r.ActionRef != nil {
		return newBulletmlError(fmt.Sprintf("Both <%s> and <%s> exist in <%s> element", r.Action.XMLName.Local, r.ActionRef.XMLName.Local, r.XMLName.Local), r)
	}

	if r.Action != nil {
		r.Action.parentNode = r
		if err := r.Action.prepare(); err != nil {
			return err
		}
	}

	if r.ActionRef != nil {
		r.ActionRef.parentNode = r
		if err := r.ActionRef.prepare(); err != nil {
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

type DirectionType string

const (
	DirectionTypeAim      DirectionType = "aim"
	DirectionTypeAbsolute DirectionType = "absolute"
	DirectionTypeRelative DirectionType = "relative"
	DirectionTypeSequence DirectionType = "sequence"
)

type Direction struct {
	XMLName    xml.Name      `xml:"direction"`
	Type       DirectionType `xml:"type,attr"`
	Expr       string        `xml:",chardata"`
	parentNode node          `xml:"-"`
}

func (d *Direction) prepare() error {
	if d.Type == "" {
		d.Type = DirectionTypeAim
	}
	if !isIn(d.Type, []DirectionType{DirectionTypeAim, DirectionTypeAbsolute, DirectionTypeRelative, DirectionTypeSequence}) {
		return newBulletmlError(fmt.Sprintf("Invalid 'type' attribute value of <%s> element: %s", d.XMLName.Local, d.Type), d)
	}

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
	XMLName    xml.Name  `xml:"speed"`
	Type       SpeedType `xml:"type,attr"`
	Expr       string    `xml:",chardata"`
	parentNode node      `xml:"-"`
}

func (s *Speed) prepare() error {
	if s.Type == "" {
		s.Type = SpeedTypeAbsolute
	}
	if !isIn(s.Type, []SpeedType{SpeedTypeAbsolute, SpeedTypeRelative, SpeedTypeSequence}) {
		return newBulletmlError(fmt.Sprintf("Invalid 'type' attribute value of <%s> element: %s", s.XMLName.Local, s.Type), s)
	}

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
	XMLName    xml.Name       `xml:"horizontal"`
	Type       HorizontalType `xml:"type,attr"`
	Expr       string         `xml:",chardata"`
	parentNode node           `xml:"-"`
}

func (h *Horizontal) prepare() error {
	if h.Type == "" {
		h.Type = HorizontalTypeAbsolute
	}
	if !isIn(h.Type, []HorizontalType{HorizontalTypeAbsolute, HorizontalTypeRelative, HorizontalTypeSequence}) {
		return newBulletmlError(fmt.Sprintf("Invalid 'type' attribute value of <%s> element: %s", h.XMLName.Local, h.Type), h)
	}

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
	XMLName    xml.Name     `xml:"vertical"`
	Type       VerticalType `xml:"type,attr"`
	Expr       string       `xml:",chardata"`
	parentNode node         `xml:"-"`
}

func (v *Vertical) prepare() error {
	if v.Type == "" {
		v.Type = VerticalTypeAbsolute
	}
	if !isIn(v.Type, []VerticalType{VerticalTypeAbsolute, VerticalTypeRelative, VerticalTypeSequence}) {
		return newBulletmlError(fmt.Sprintf("Invalid 'type' attribute value of <%s> element: %s", v.XMLName.Local, v.Type), v)
	}

	return nil
}

func (v *Vertical) parent() node {
	return v.parentNode
}

func (v *Vertical) xmlName() string {
	return v.XMLName.Local
}

type Term struct {
	XMLName    xml.Name `xml:"term"`
	Expr       string   `xml:",chardata"`
	parentNode node     `xml:"-"`
}

func (t *Term) prepare() error {
	return nil
}

func (t *Term) parent() node {
	return t.parentNode
}

func (t *Term) xmlName() string {
	return t.XMLName.Local
}

type Times struct {
	XMLName    xml.Name `xml:"times"`
	Expr       string   `xml:",chardata"`
	parentNode node     `xml:"-"`
}

func (t *Times) prepare() error {
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
	Params     []Param  `xml:"param"`
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

type ActionRef struct {
	XMLName    xml.Name `xml:"actionRef"`
	Label      string   `xml:"label,attr"`
	Params     []Param  `xml:"param"`
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

type FireRef struct {
	XMLName    xml.Name `xml:"fireRef"`
	Label      string   `xml:"label,attr"`
	Params     []Param  `xml:"param"`
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

type Param struct {
	XMLName    xml.Name `xml:"param"`
	Expr       string   `xml:",chardata"`
	parentNode node     `xml:"-"`
}

func (p *Param) prepare() error {
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
