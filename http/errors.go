package http

import (
	"github.com/goodbye-jack/go-common/log"
	"github.com/pkg/errors"
)

const (
	serverErrorMessage    = "服务器有点儿累, 稍作休息."
	clientErrorMessage    = "操作问题"
	paramsErrorMessage    = "输入参数有问题， 要检查一下输入参数"
	intervalErrorMessage  = "服务器出了点问题，联系服务器维护人员吧"
	duplicateErrorMessage = "相关数据已经存在于系统内啦."
	wrongPassErrorMessage = "手机号或者密码错误."
)

type serverError struct {
	message string
}

type clientError struct {
	message string
}

type paramsError struct {
	message string
}

type intervalError struct {
	message string
}

type duplicateError struct {
	message string
}

type wrongPassError struct {
	message string
}

func (e serverError) Error() string {
	return e.message
}

func ServerError(message string) error {
	err := serverError{}
	if message == "" {
		err.message = serverErrorMessage
	}
	err.message = message
	return errors.Wrap(err, message)
}

func ServerErrorf(format string, opt ...interface{}) error {
	err := serverError{
		message: serverErrorMessage,
	}
	return errors.Wrapf(err, format, opt...)
}

func (e clientError) Error() string {
	return e.message
}

func ClientError(message string) error {
	err := clientError{}
	if message == "" {
		message = clientErrorMessage
	}
	err.message = message
	return errors.Wrapf(err, message)
}

func ClientErrorf(format string, opt ...interface{}) error {
	err := clientError{
		message: clientErrorMessage,
	}
	return errors.Wrapf(err, format, opt...)
}

func (e paramsError) Error() string {
	return e.message
}

func ParamsError(message string) error {
	err := paramsError{}
	if message == "" {
		message = paramsErrorMessage
	}
	err.message = message
	return errors.Wrapf(err, message)
}

func ParamsErrorf(format string, opt ...interface{}) error {
	err := paramsError{
		message: paramsErrorMessage,
	}
	return errors.Wrapf(err, format, opt...)
}

func (e intervalError) Error() string {
	return e.message
}

func IntervalError(message string) error {
	err := intervalError{}
	if message == "" {
		message = intervalErrorMessage
	}
	err.message = message
	return errors.Wrapf(err, message)
}

func IntervalErrorf(format string, opt ...interface{}) error {
	err := intervalError{
		message: intervalErrorMessage,
	}
	return errors.Wrapf(err, format, opt...)
}

func (e duplicateError) Error() string {
	return e.message
}

func DuplicateError(message string) error {
	err := duplicateError{
		message: duplicateErrorMessage,
	}
	return errors.Wrapf(err, message)
}

func DuplicateErrorf(format string, opt ...interface{}) error {
	err := duplicateError{
		message: duplicateErrorMessage,
	}
	return errors.Wrapf(err, format, opt...)
}

func (e wrongPassError) Error() string {
	return e.message
}

func WrongPassError(message string) error {
	err := wrongPassError{
		message: wrongPassErrorMessage,
	}
	return errors.Wrapf(err, message)
}

func WrongPassErrorf(format string, opt ...interface{}) error {
	err := wrongPassError{
		message: wrongPassErrorMessage,
	}
	return errors.Wrapf(err, format, opt...)
}
func whichError(err error) string {
	log.Error("http error, %v", err)
	for _, httpError := range []error{
		&serverError{
			message: serverErrorMessage,
		},
		&clientError{
			message: clientErrorMessage,
		},
		&paramsError{
			message: paramsErrorMessage,
		},
		&intervalError{
			message: intervalErrorMessage,
		},
		&duplicateError{
			message: duplicateErrorMessage,
		},
		&wrongPassError{
			message: wrongPassErrorMessage,
		},
	} {
		if errors.As(err, httpError) {
			log.Info(err)
			log.Info("whichError, return %s", httpError.Error())
			return httpError.Error()
		}
	}
	log.Warn("whichError(%v) not match. go default", err)
	return err.Error()
}
