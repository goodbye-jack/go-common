package log

import (
	"fmt"
	"github.com/common-nighthawk/go-figure"
	"github.com/sirupsen/logrus"
	"os"
	"strings"
)

func LoadPrintProjectName(projectName string) {
	bannerMode := strings.ToLower(strings.TrimSpace(os.Getenv("GO_COMMON_LOG_BANNER")))
	if bannerMode == "off" || bannerMode == "false" || bannerMode == "0" {
		return
	}
	// 生成ASCII Art 字体可选："slant"、"small"、"doom"、“epic”、“isometric1”、“larry3d”、“big”、“block”、“colossal”、“graffiti”、“cyberlarge”、“speed”、“rectangles”等（推荐slant） 选一个更紧凑的字体（比如slant）
	myFigure := figure.NewFigure(projectName, "colossal", true)
	_, _ = fmt.Fprintln(logrus.StandardLogger().Out, "\n"+myFigure.String())
}

func Debug(args ...interface{}) {
	logrus.Debug(normalizeArgs(args...))
}

func Debugf(format string, args ...interface{}) {
	logrus.Debugf(format, args...)
}

func Info(args ...interface{}) {
	logrus.Info(normalizeArgs(args...))
}

func Infof(format string, args ...interface{}) {
	logrus.Infof(format, args...)
}

func Warn(args ...interface{}) {
	logrus.Warn(normalizeArgs(args...))
}

func Warnf(format string, args ...interface{}) {
	logrus.Warnf(format, args...)
}

func Error(args ...interface{}) {
	logrus.Error(normalizeArgs(args...))
}

func Errorf(format string, args ...interface{}) {
	logrus.Errorf(format, args...)
}

func Panic(args ...interface{}) {
	logrus.Panic(normalizeArgs(args...))
}

func Panicf(format string, args ...interface{}) {
	logrus.Panicf(format, args...)
}

func Fatal(args ...interface{}) {
	logrus.Fatal(normalizeArgs(args...))
}

func normalizeArgs(args ...interface{}) string {
	if len(args) == 0 {
		return ""
	}
	if format, ok := args[0].(string); ok {
		if len(args) > 1 && strings.Contains(format, "%") {
			return fmt.Sprintf(format, args[1:]...)
		}
		return fmt.Sprint(args...)
	}
	return fmt.Sprint(args...)
}

func Fatalf(format string, args ...interface{}) {
	logrus.Fatalf(format, args...)
}
