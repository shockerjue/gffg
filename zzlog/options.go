package zzlog

type LoggerOption func(*options)

type options struct {
	level   string
	logName string
}

// Set the log output level
//
// @param	level 	Default:info
//
//	debug,info,warn,error,dpanic,panic,fatal
func WithLevel(level string) LoggerOption {
	return func(args *options) {
		args.level = level
	}
}

// The name of the log file to save
//
// @param	logName 	file name
func WithLogName(logName string) LoggerOption {
	return func(args *options) {
		args.logName = logName
	}
}

func initOpts(opts ...LoggerOption) *options {
	var opt options
	for _, o := range opts {
		o(&opt)
	}

	return &opt
}
