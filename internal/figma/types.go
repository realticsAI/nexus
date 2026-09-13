package figma

type File struct {
	Key          string     `json:"key"`
	Name         string     `json:"name"`
	LastModified string     `json:"lastModified"`
	Version      string     `json:"version"`
	Document     *Node      `json:"document"`
	Components   Components `json:"components"`
}

type Node struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Children   []*Node `json:"children,omitempty"`
	Characters string  `json:"characters,omitempty"`

	AbsoluteBoundingBox *Rect   `json:"absoluteBoundingBox,omitempty"`
	Fills               []Paint `json:"fills,omitempty"`
	Strokes             []Paint `json:"strokes,omitempty"`
	StrokeWeight        float64 `json:"strokeWeight,omitempty"`
	CornerRadius        float64 `json:"cornerRadius,omitempty"`
	Opacity             float64 `json:"opacity,omitempty"`

	Style         map[string]any `json:"style,omitempty"`
	LayoutMode    string         `json:"layoutMode,omitempty"`
	PrimaryAxisSizingMode   string `json:"primaryAxisSizingMode,omitempty"`
	CounterAxisSizingMode   string `json:"counterAxisSizingMode,omitempty"`
	PaddingLeft   float64 `json:"paddingLeft,omitempty"`
	PaddingRight  float64 `json:"paddingRight,omitempty"`
	PaddingTop    float64 `json:"paddingTop,omitempty"`
	PaddingBottom float64 `json:"paddingBottom,omitempty"`
	ItemSpacing   float64 `json:"itemSpacing,omitempty"`
}

type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Paint struct {
	Type    string `json:"type"`
	Visible *bool  `json:"visible,omitempty"`
	Color   *Color `json:"color,omitempty"`
}

type Color struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
	A float64 `json:"a"`
}

type Components map[string]ComponentMeta

type ComponentMeta struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ImageResponse struct {
	Images map[string]string `json:"images"`
	Err    string            `json:"err,omitempty"`
}

type DesignContext struct {
	FileName   string           `json:"file_name"`
	FileKey    string           `json:"file_key"`
	Pages      []PageSummary    `json:"pages"`
	Components []ComponentInfo  `json:"components"`
	Frames     []FrameSummary   `json:"frames"`
}

type PageSummary struct {
	Name       string `json:"name"`
	FrameCount int    `json:"frame_count"`
}

type ComponentInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	NodeID      string `json:"node_id"`
}

type FrameSummary struct {
	Name     string  `json:"name"`
	NodeID   string  `json:"node_id"`
	Page     string  `json:"page"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
	Children int     `json:"children"`
	Text     []string `json:"text,omitempty"`
}
