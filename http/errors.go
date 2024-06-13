package http

import (
	"github.com/goodbye-jack/go-common/log"
	"github.com/pkg/errors"
)

const (
	serverErrorMessage = "服务器有点儿累, 稍作休息."
	clientErrorMessage = "操作问题"
	paramsErrorMessage = "输入参数有问题， 要检查一下输入参数"
	intervalErrorMessage = "服务器出了点问题，联系服务器维护人员吧"
	duplicateErrorMessage = "相关数据已经存在于系统内啦."
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

type duplicateError struct{
	message string
}

func (e serverError) Error() string {
	return e.message
}

func ServerError(message string) error {
	err := serverError{
		message: "serverError",
	}
	return errors.Wrap(err, message)
}

func ServerErrorf(format string, opt ...interface{}) error {
	err := serverError{
		message: "serverError",
	}
	return errors.Wrapf(err, format, opt...)
}

func (e clientError) Error() string {
	return e.message
}

func ClientError(message string) error {
	err := clientError{
		message: "clientError",
	}
	return errors.Wrapf(err, message)
}

func ClientErrorf(format string, opt ...interface{}) error {
	err := clientError{
		message: "clientError",
	}
	return errors.Wrapf(err, format, opt...)
}

func (e paramsError) Error() string {
	return e.message
}

func ParamsError(message string) error {
	err := paramsError{
		message: "paramsError",
	}
	return errors.Wrapf(err, message)
}

func ParamsErrorf(format string, opt ...interface{}) error {
	err := paramsError{
		message: "paramsError",
	}
	return errors.Wrapf(err, format, opt...)
}

func (e intervalError) Error() string {
	return e.message
}

func IntervalError(message string) error {
	err := intervalError{
		message: "intervalError",
	}
	return errors.Wrapf(err, message)
}

func IntervalErrorf(format string, opt ...interface{}) error {
	err := intervalError{
		message: "intervalError",
	}
	return errors.Wrapf(err, format, opt...)
}

func (e duplicateError) Error() string {
	return e.message
}

func DuplicateError(message string) error {
	err := duplicateError{
		message: "duplicateError",
	}
	return errors.Wrapf(err, message)
}

func DuplicateErrorf(format string, opt ...interface{}) error {
	err := duplicateError{
		message: "duplicateError",
	}
	return errors.Wrapf(err, format, opt...)
}

func whichError(err error) string {
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
	} {
		log.Info(httpError)
		if errors.As(err, httpError) {
			return httpError.Error()
		}
	}
	log.Warn("whichError(%v) not match. go default", err)
	return err.Error()
}
