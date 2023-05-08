package bulletml

import (
	"encoding/xml"
	"fmt"
	"io"
)

func Load(src io.Reader) (*BulletML, error) {
	var b BulletML
	if err := xml.NewDecoder(src).Decode(&b); err != nil {
		return nil, err
	}
	return &b, nil
}

func decodeElement[T any](d *xml.Decoder, start *xml.StartElement) (T, error) {
	var v T
	if err := d.DecodeElement(&v, start); err != nil {
		return v, err
	}
	return v, nil
}

type BulletMLType string

const (
	BulletMLTypeNone       BulletMLType = "none"
	BulletMLTypeVertical   BulletMLType = "vertical"
	BulletMLTypeHorizontal BulletMLType = "horizontal"
)

type BulletML struct {
	XMLName  xml.Name     `xml:"bulletml"`
	Type     BulletMLType `xml:"type,attr"`
	Contents []any
}

func (b *BulletML) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	b.XMLName = start.Name

	b.Type = BulletMLTypeNone
	for _, attr := range start.Attr {
		if attr.Name.Local == "type" {
			switch attr.Value {
			case "none":
				b.Type = BulletMLTypeNone
			case "vertical":
				b.Type = BulletMLTypeVertical
			case "horizontal":
				b.Type = BulletMLTypeHorizontal
			default:
				return fmt.Errorf("Invalid value '%s' for 'type' attribute of <bulletml>", attr.Value)
			}
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
			case "bullet":
				if e, err = decodeElement[Bullet](d, &s); err != nil {
					return err
				}
			case "action":
				if e, err = decodeElement[Action](d, &s); err != nil {
					return err
				}
			case "fire":
				if e, err = decodeElement[Fire](d, &s); err != nil {
					return err
				}
			default:
				return fmt.Errorf("Unexpected element <%s> in <bulletml>", s.Name.Local)
			}
			b.Contents = append(b.Contents, e)
		}
	}

	return nil
}

type Bullet struct {
	XMLName   xml.Name   `xml:"bullet"`
	Label     string     `xml:"label,attr,omitempty"`
	Direction *Direction `xml:"direction,omitempty"`
	Speed     *Speed     `xml:"speed,omitempty"`
	Contents  []any
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
					b.Contents = append(b.Contents, a)
				}
			case "actionRef":
				if a, err := decodeElement[ActionRef](d, &s); err != nil {
					return err
				} else {
					b.Contents = append(b.Contents, a)
				}
			default:
				return fmt.Errorf("Unexpected element <%s> in <bullet>", s.Name.Local)
			}
		}
	}

	return nil
}

type Action struct {
	XMLName  xml.Name `xml:"action"`
	Label    string   `xml:"label,attr,omitempty"`
	Contents []any
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
			a.Contents = append(a.Contents, e)
		}
	}

	return nil
}

type Fire struct {
	XMLName     xml.Name   `xml:"fire"`
	Label       string     `xml:"label,attr,omitempty"`
	Direction   *Direction `xml:"direction,omitempty"`
	Speed       *Speed     `xml:"speed,omitempty"`
	BulletOrRef any
}

func (f *Fire) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	f.XMLName = start.Name

	for _, attr := range start.Attr {
		if attr.Name.Local == "label" {
			f.Label = attr.Value
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
					f.Direction = &d
				}
			case "speed":
				if sp, err := decodeElement[Speed](d, &s); err != nil {
					return err
				} else {
					f.Speed = &sp
				}
			case "bullet":
				if b, err := decodeElement[Bullet](d, &s); err != nil {
					return err
				} else {
					f.BulletOrRef = b
				}
			case "bulletRef":
				if b, err := decodeElement[BulletRef](d, &s); err != nil {
					return err
				} else {
					f.BulletOrRef = b
				}
			default:
				return fmt.Errorf("Unexpected element <%s> in <fire>", s.Name.Local)
			}
		}
	}

	return nil
}

type ChangeDirection struct {
	XMLName   xml.Name  `xml:"changeDirection"`
	Direction Direction `xml:"direction"`
	Term      Term      `xml:"term"`
}

func (c *ChangeDirection) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	c.XMLName = start.Name

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
					c.Direction = d
				}
			case "term":
				if t, err := decodeElement[Term](d, &s); err != nil {
					return err
				} else {
					c.Term = t
				}
			default:
				return fmt.Errorf("Unexpected element <%s> in <changeDirection>", s.Name.Local)
			}
		}
	}

	return nil
}

type ChangeSpeed struct {
	XMLName xml.Name `xml:"changeSpeed"`
	Speed   Speed    `xml:"speed"`
	Term    Term     `xml:"term"`
}

func (c *ChangeSpeed) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	c.XMLName = start.Name

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
			case "speed":
				if sp, err := decodeElement[Speed](d, &s); err != nil {
					return err
				} else {
					c.Speed = sp
				}
			case "term":
				if t, err := decodeElement[Term](d, &s); err != nil {
					return err
				} else {
					c.Term = t
				}
			default:
				return fmt.Errorf("Unexpected element <%s> in <changeSpeed>", s.Name.Local)
			}
		}
	}

	return nil
}

type Accel struct {
	XMLName    xml.Name    `xml:"accel"`
	Horizontal *Horizontal `xml:"horizontal,omitempty"`
	Vertical   *Vertical   `xml:"vertical,omitempty"`
	Term       Term        `xml:"term"`
}

func (a *Accel) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	a.XMLName = start.Name

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
			case "horizontal":
				if h, err := decodeElement[Horizontal](d, &s); err != nil {
					return err
				} else {
					a.Horizontal = &h
				}
			case "vertical":
				if v, err := decodeElement[Vertical](d, &s); err != nil {
					return err
				} else {
					a.Vertical = &v
				}
			case "term":
				if t, err := decodeElement[Term](d, &s); err != nil {
					return err
				} else {
					a.Term = t
				}
			default:
				return fmt.Errorf("Unexpected element <%s> in <accel>", s.Name.Local)
			}
		}
	}

	return nil
}

type Wait struct {
	XMLName xml.Name `xml:"wait"`
	Expr    string   `xml:",innerxml"`
}

type Vanish struct {
	XMLName xml.Name `xml:"vanish"`
}

type Repeat struct {
	XMLName     xml.Name `xml:"repeat"`
	Times       Times    `xml:"times"`
	ActionOrRef any
}

func (r *Repeat) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	r.XMLName = start.Name

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
			case "times":
				if t, err := decodeElement[Times](d, &s); err != nil {
					return err
				} else {
					r.Times = t
				}
			case "action":
				if a, err := decodeElement[Action](d, &s); err != nil {
					return err
				} else {
					r.ActionOrRef = a
				}
			case "actionRef":
				if a, err := decodeElement[ActionRef](d, &s); err != nil {
					return err
				} else {
					r.ActionOrRef = a
				}
			default:
				return fmt.Errorf("Unexpected element <%s> in <repeat>", s.Name.Local)
			}
		}
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
	XMLName xml.Name      `xml:"direction"`
	Type    DirectionType `xml:"type,attr"`
	Expr    string        `xml:",innerxml"`
}

func (dir *Direction) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	dir.XMLName = start.Name

	for _, attr := range start.Attr {
		if attr.Name.Local == "type" {
			switch attr.Value {
			case "aim":
				dir.Type = DirectionTypeAim
			case "absolute":
				dir.Type = DirectionTypeAbsolute
			case "relative":
				dir.Type = DirectionTypeRelative
			case "sequence":
				dir.Type = DirectionTypeSequence
			default:
				return fmt.Errorf("Invalid value '%s' for 'type' attribute of <direction>", attr.Value)
			}
		}
	}

	if err := d.DecodeElement(&dir.Expr, &start); err != nil {
		return err
	}

	return nil
}

type SpeedType string

const (
	SpeedTypeAbsolute SpeedType = "absolute"
	SpeedTypeRelative SpeedType = "relative"
	SpeedTypeSequence SpeedType = "sequence"
)

type Speed struct {
	XMLName xml.Name  `xml:"speed"`
	Type    SpeedType `xml:"type,attr"`
	Expr    string    `xml:",innerxml"`
}

func (s *Speed) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	s.XMLName = start.Name

	for _, attr := range start.Attr {
		if attr.Name.Local == "type" {
			switch attr.Value {
			case "absolute":
				s.Type = SpeedTypeAbsolute
			case "relative":
				s.Type = SpeedTypeRelative
			case "sequence":
				s.Type = SpeedTypeSequence
			default:
				return fmt.Errorf("Invalid value '%s' for 'type' attribute of <speed>", attr.Value)
			}
		}
	}

	if err := d.DecodeElement(&s.Expr, &start); err != nil {
		return err
	}

	return nil
}

type HorizontalType string

const (
	HorizontalTypeAbsolute HorizontalType = "absolute"
	HorizontalTypeRelative HorizontalType = "relative"
	HorizontalTypeSequence HorizontalType = "sequence"
)

type Horizontal struct {
	XMLName xml.Name       `xml:"horizontal"`
	Type    HorizontalType `xml:"type,attr"`
	Expr    string         `xml:",innerxml"`
}

func (h *Horizontal) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	h.XMLName = start.Name

	for _, attr := range start.Attr {
		if attr.Name.Local == "type" {
			switch attr.Value {
			case "absolute":
				h.Type = HorizontalTypeAbsolute
			case "relative":
				h.Type = HorizontalTypeRelative
			case "sequence":
				h.Type = HorizontalTypeSequence
			default:
				return fmt.Errorf("Invalid value '%s' for 'type' attribute of <horizontal>", attr.Value)
			}
		}
	}

	if err := d.DecodeElement(&h.Expr, &start); err != nil {
		return err
	}

	return nil
}

type VerticalType string

const (
	VerticalTypeAbsolute VerticalType = "absolute"
	VerticalTypeRelative VerticalType = "relative"
	VerticalTypeSequence VerticalType = "sequence"
)

type Vertical struct {
	XMLName xml.Name     `xml:"vertical"`
	Type    VerticalType `xml:"type,attr"`
	Expr    string       `xml:",innerxml"`
}

func (v *Vertical) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	v.XMLName = start.Name

	for _, attr := range start.Attr {
		if attr.Name.Local == "type" {
			switch attr.Value {
			case "absolute":
				v.Type = VerticalTypeAbsolute
			case "relative":
				v.Type = VerticalTypeRelative
			case "sequence":
				v.Type = VerticalTypeSequence
			default:
				return fmt.Errorf("Invalid value '%s' for 'type' attribute of <vertical>", attr.Value)
			}
		}
	}

	if err := d.DecodeElement(&v.Expr, &start); err != nil {
		return err
	}

	return nil
}

type Term struct {
	XMLName xml.Name `xml:"term"`
	Expr    string   `xml:",innerxml"`
}

type Times struct {
	XMLName xml.Name `xml:"times"`
	Expr    string   `xml:",innerxml"`
}

type BulletRef struct {
	XMLName xml.Name `xml:"bulletRef"`
	Label   string   `xml:"label,attr"`
	Params  []Param
}

func (b *BulletRef) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
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
			case "param":
				if p, err := decodeElement[Param](d, &s); err != nil {
					return err
				} else {
					b.Params = append(b.Params, p)
				}
			default:
				return fmt.Errorf("Unexpected element <%s> in <bulletRef>", s.Name.Local)
			}
		}
	}

	return nil
}

type ActionRef struct {
	XMLName xml.Name `xml:"actionRef"`
	Label   string   `xml:"label,attr"`
	Params  []Param
}

func (a *ActionRef) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
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
			switch s.Name.Local {
			case "param":
				if p, err := decodeElement[Param](d, &s); err != nil {
					return err
				} else {
					a.Params = append(a.Params, p)
				}
			default:
				return fmt.Errorf("Unexpected element <%s> in <actionRef>", s.Name.Local)
			}
		}
	}

	return nil
}

type FireRef struct {
	XMLName xml.Name `xml:"fireRef"`
	Label   string   `xml:"label,attr"`
	Params  []Param
}

func (f *FireRef) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	f.XMLName = start.Name

	for _, attr := range start.Attr {
		if attr.Name.Local == "label" {
			f.Label = attr.Value
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
			case "param":
				if p, err := decodeElement[Param](d, &s); err != nil {
					return err
				} else {
					f.Params = append(f.Params, p)
				}
			default:
				return fmt.Errorf("Unexpected element <%s> in <fireRef>", s.Name.Local)
			}
		}
	}

	return nil
}

type Param struct {
	XMLName xml.Name `xml:"param"`
	Expr    string   `xml:",innerxml"`
}
