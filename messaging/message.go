package messaging

type Message struct {
	Id         string
	Attributes map[string]any
	Data       string
}
