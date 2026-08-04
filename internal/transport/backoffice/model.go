package backoffice

type PageData struct {
	Title  string
	Data   any
	Filter any
	CSRF   string
}
