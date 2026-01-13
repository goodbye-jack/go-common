package log

import (
	"fmt"
	"github.com/common-nighthawk/go-figure"
	"github.com/sirupsen/logrus"
)

// 存储项目名称
//var projectName string

//// Init 业务项目调用时传递项目名，动态生成ASCII Art
//func Init(projectName string) {
//	// 生成项目名的ASCII Art
//	myFigure := figure.NewFigure(projectName, "", true)
//	// 打印ASCII Art
//	logrus.Info("\n" + myFigure.String())
//	// 打印启动日志（和图里格式一致）
//	logrus.Infof("[main] [INFO] %s -- The following profiles are active: dev", projectName)
//}

func Init(projectName string) {
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
