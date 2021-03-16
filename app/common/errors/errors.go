package errors

import "strings"

const (
	EmptyFieldErr   = "обязательные поля не заполнены или содержат недопустимые данные"
	ServerErr       = "что-то пошло не так. Попробуйте позже"
	NotFound        = "по данному запросу ничего не нашлось"
)

type Err interface {
	Msg() string
	StatusCode() int
}

type RespErr struct {
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (r RespErr) Msg() string {
	return r.Message
}

func (r RespErr) StatusCode() int {
	return r.Status
}

func NewErr(statusCode int, message string) Err {
	return RespErr{Status: statusCode, Message: message}
}

func RecordExists(errMsg string) bool {
	return strings.Contains(errMsg, "duplicate")
}

func NoRows(errMsg string) bool {
	return strings.Contains(errMsg, "no rows") || strings.Contains(errMsg, "record not")
}

func TimeoutCheck(errMsg string) bool {
	return strings.Contains(errMsg, "Client.Timeout exceeded")
}

