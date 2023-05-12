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
	XMLName xml.Name     `xml:"bulletml"`
	Type    BulletMLType `xml:"type,attr"`
	Bullets []Bullet     `xml:"bullet"`
	Actions []Action     `xml:"action"`
	Fires   []Fire       `xml:"fire"`
}

type Bullet struct {
	XMLName      xml.Name   `xml:"bullet"`
	Label        string     `xml:"label,attr,omitempty"`
	Direction    *Direction `xml:"direction,omitempty"`
	Speed        *Speed     `xml:"speed,omitempty"`
	ActionOrRefs []any      `xml:",any"`
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
	XMLName  xml.Name `xml:"action"`
	Label    string   `xml:"label,attr,omitempty"`
	Commands []any    `xml:",any"`
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
	XMLName   xml.Name   `xml:"fire"`
	Label     string     `xml:"label,attr,omitempty"`
	Direction *Direction `xml:"direction,omitempty"`
	Speed     *Speed     `xml:"speed,omitempty"`
	Bullet    *Bullet    `xml:"bullet,omitempty"`
	BulletRef *BulletRef `xml:"bulletRef,omitempty"`
}

type ChangeDirection struct {
	XMLName   xml.Name  `xml:"changeDirection"`
	Direction Direction `xml:"direction"`
	Term      Term      `xml:"term"`
}

type ChangeSpeed struct {
	XMLName xml.Name `xml:"changeSpeed"`
	Speed   Speed    `xml:"speed"`
	Term    Term     `xml:"term"`
}

type Accel struct {
	XMLName    xml.Name    `xml:"accel"`
	Horizontal *Horizontal `xml:"horizontal,omitempty"`
	Vertical   *Vertical   `xml:"vertical,omitempty"`
	Term       Term        `xml:"term"`
}

type Wait struct {
	XMLName xml.Name `xml:"wait"`
	Expr    string   `xml:",chardata"`
}

type Vanish struct {
	XMLName xml.Name `xml:"vanish"`
}

type Repeat struct {
	XMLName   xml.Name   `xml:"repeat"`
	Times     Times      `xml:"times"`
	Action    *Action    `xml:"action,omitempty"`
	ActionRef *ActionRef `xml:"actionRef,omitempty"`
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
	Expr    string        `xml:",chardata"`
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
	Expr    string    `xml:",chardata"`
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
	Expr    string         `xml:",chardata"`
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
	Expr    string       `xml:",chardata"`
}

type Term struct {
	XMLName xml.Name `xml:"term"`
	Expr    string   `xml:",chardata"`
}

type Times struct {
	XMLName xml.Name `xml:"times"`
	Expr    string   `xml:",chardata"`
}

type BulletRef struct {
	XMLName xml.Name `xml:"bulletRef"`
	Label   string   `xml:"label,attr"`
	Params  []Param  `xml:"param"`
}

func (b BulletRef) xmlName() string {
	return b.XMLName.Local
}

func (b BulletRef) label() string {
	return b.Label
}

func (b BulletRef) params() []Param {
	return b.Params
}

type ActionRef struct {
	XMLName xml.Name `xml:"actionRef"`
	Label   string   `xml:"label,attr"`
	Params  []Param  `xml:"param"`
}

func (a ActionRef) xmlName() string {
	return a.XMLName.Local
}

func (a ActionRef) label() string {
	return a.Label
}

func (a ActionRef) params() []Param {
	return a.Params
}

type FireRef struct {
	XMLName xml.Name `xml:"fireRef"`
	Label   string   `xml:"label,attr"`
	Params  []Param  `xml:"param"`
}

func (f FireRef) xmlName() string {
	return f.XMLName.Local
}

func (f FireRef) label() string {
	return f.Label
}

func (f FireRef) params() []Param {
	return f.Params
}

type Param struct {
	XMLName xml.Name `xml:"param"`
	Expr    string   `xml:",chardata"`
}

type refType interface {
	xmlName() string
	label() string
	params() []Param
}
