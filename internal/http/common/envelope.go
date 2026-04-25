package common

type Envelope struct {
	Data  any    `json:"data,omitempty"`
	Meta  *Meta  `json:"meta,omitempty"`
	Error *Error `json:"error,omitempty"`
}

type Meta struct {
	NextCursor string `json:"next_cursor,omitempty"`
}

type Error struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func Data(v any) Envelope                 { return Envelope{Data: v} }
func DataWithMeta(v any, m Meta) Envelope { return Envelope{Data: v, Meta: &m} }
func Err(code int, msg string) Envelope   { return Envelope{Error: &Error{Code: code, Message: msg}} }
