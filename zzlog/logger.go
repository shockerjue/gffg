package zzlog

import (
	"io"
	"os"

	"github.com/polarismesh/polaris-go/api"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

var (
	defLogger *zap.SugaredLogger
)

func init() {
	logger, _ := zap.NewProduction()
	defLogger = logger.Sugar()
	if err := api.ConfigLoggers("", api.NoneLog); err != nil {
		// do error handle
	}

	return
}

// Initialize log configuration
func Init(opts ...LoggerOption) {
	opt := initOpts(opts...)
	if 0 == len(opt.logName) {
		return
	}

	var level zapcore.Level
	level, err := zapcore.ParseLevel(opt.level)
	if nil != err {
		level = zapcore.InfoLevel
	}

	jacklog := &lumberjack.Logger{
		Filename:   opt.logName,
		MaxSize:    16, // megabytes
		MaxBackups: 3,
		MaxAge:     7,    //days
		Compress:   true, // disabled by default
	}

	// defer jacklog.Close()
	multi := io.MultiWriter(os.Stdout, jacklog)

	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder //
	fileEncoder := zapcore.NewJSONEncoder(config)
	core := zapcore.NewCore(
		fileEncoder,            //
		zapcore.AddSync(multi), //
		level,                  //
	)
	logger := zap.New(core, zap.AddCallerSkip(1), zap.AddCaller())
	// defer logger.Sync() //
	defLogger = logger.Sugar()
}

func DPanic(args ...interface{}) {
	defLogger.DPanic(args...)
}

func DPanicf(template string, args ...interface{}) {
	defLogger.DPanicf(template, args...)
}

func DPanicln(args ...interface{}) {
	defLogger.DPanicln(args...)
}

func DPanicw(msg string, keysAndValues ...interface{}) {
	defLogger.DPanicw(msg, keysAndValues...)
}

func Debug(args ...interface{}) {
	defLogger.Debug(args...)
}

func Debugf(template string, args ...interface{}) {
	defLogger.Debugf(template, args...)
}

func Debugln(args ...interface{}) {
	defLogger.Debugln(args...)
}

func Debugw(msg string, keysAndValues ...interface{}) {
	defLogger.Debugw(msg, keysAndValues...)
}

func Error(args ...interface{}) {
	defLogger.Error(args...)
}

func Errorf(template string, args ...interface{}) {
	defLogger.Errorf(template, args...)
}

func Errorln(args ...interface{}) {
	defLogger.Errorln(args...)
}

func Errorw(msg string, keysAndValues ...interface{}) {
	defLogger.Errorw(msg, keysAndValues...)
}

func Fatal(args ...interface{}) {
	defLogger.Fatal(args...)
}

func Fatalf(template string, args ...interface{}) {
	defLogger.Fatalf(template, args...)
}

func Fatalln(args ...interface{}) {
	defLogger.Fatalln(args...)
}

func Fatalw(msg string, keysAndValues ...interface{}) {
	defLogger.Fatalw(msg, keysAndValues...)
}

func Info(args ...interface{}) {
	defLogger.Info(args...)
}

func Infof(template string, args ...interface{}) {
	defLogger.Infof(template, args...)
}

func Infoln(args ...interface{}) {
	defLogger.Infoln(args...)
}

func Infow(msg string, keysAndValues ...interface{}) {
	defLogger.Infow(msg, keysAndValues...)
}

func Panic(args ...interface{}) {
	defLogger.Panic(args...)
}

func Panicf(template string, args ...interface{}) {
	defLogger.Panicf(template, args...)
}

func Panicln(args ...interface{}) {
	defLogger.Panicln(args...)
}

func Panicw(msg string, keysAndValues ...interface{}) {
	defLogger.Panicw(msg, keysAndValues...)
}

func Warn(args ...interface{}) {
	defLogger.Warn(args...)
}

func Warnf(template string, args ...interface{}) {
	defLogger.Warnf(template, args...)
}

func Warnln(args ...interface{}) {
	defLogger.Warnln(args...)
}

func Warnw(msg string, keysAndValues ...interface{}) {
	defLogger.Warnw(msg, keysAndValues...)
}
