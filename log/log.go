package log

import (
	"fmt"
	"github.com/common-nighthawk/go-figure"
	"github.com/sirupsen/logrus"
)

func LoadPrintProjectName(projectName string) {
	// 生成ASCII Art 字体可选："slant"、"small"、"doom"、“epic”、“isometric1”、“larry3d”、“big”、“block”、“colossal”、“graffiti”、“cyberlarge”、“speed”、“rectangles”等（推荐slant） 选一个更紧凑的字体（比如slant）
	myFigure := figure.NewFigure(projectName, "colossal", true)
	fmt.Println("\n" + myFigure.String())
}

func Debug(args ...interface{}) {
	logrus.Debug(args)
}

func Debugf(format string, args ...interface{}) {
	logrus.Debugf(format, args)
}

func Info(args ...interface{}) {
	logrus.Info(args)
}

func Infof(format string, args ...interface{}) {
	logrus.Infof(format, args)
}

func Warn(args ...interface{}) {
	logrus.Warn(args)
}

func Warnf(format string, args ...interface{}) {
	logrus.Warnf(format, args)
}

func Error(args ...interface{}) {
	logrus.Error(args)
}

func Errorf(format string, args ...interface{}) {
	logrus.Errorf(format, args)
}

func Panic(args ...interface{}) {
	logrus.Panic(args)
}

func Panicf(format string, args ...interface{}) {
	logrus.Panicf(format, args)
}

func Fatal(args ...interface{}) {
	logrus.Fatal(args)
}

func Fatalf(format string, args ...interface{}) {
	logrus.Fatalf(format, args)
}
