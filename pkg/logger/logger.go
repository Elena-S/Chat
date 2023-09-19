package logger

import (
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func NewZapLogger() *zap.Logger {
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(config)
	writer := zapcore.AddSync(&lumberjack.Logger{
		//TODO: need config file
		Filename:   "../../logs/server.log",
		MaxSize:    100,
		MaxBackups: 1,
		MaxAge:     1,
		Compress:   true,
	})
	core := zapcore.NewTee(zapcore.NewCore(fileEncoder, writer, zapcore.InfoLevel))
	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
}

type Logger struct {
	logger *zap.Logger
}

type Params struct {
	fx.In
	ZapLogger *zap.Logger
}

func New(p Params) *Logger {
	return &Logger{
		logger: p.ZapLogger,
	}
}

func (l *Logger) With(key string, data any) *Logger {
	var field zapcore.Field

	switch value := data.(type) {
	case uint:
		field = zap.Uint(key, value)
	case uint8:
		field = zap.Uint8(key, value)
	case uint16:
		field = zap.Uint16(key, value)
	case uint32:
		field = zap.Uint32(key, value)
	case uint64:
		field = zap.Uint64(key, value)
	case int:
		field = zap.Int(key, value)
	case int8:
		field = zap.Int8(key, value)
	case int16:
		field = zap.Int16(key, value)
	case int32:
		field = zap.Int32(key, value)
	case int64:
		field = zap.Int64(key, value)
	case []byte:
		field = zap.ByteString(key, value)
	default:
		field = zap.String(key, fmt.Sprint(value))
	}
	return &Logger{logger: l.logger.With(field)}
}

func (l *Logger) WithEventField(event string) *Logger {
	return l.With("event", event)
}

func (l *Logger) Info(msg string) {
	l.logger.Info(msg)
}

func (l *Logger) Error(msg string) {
	l.logger.Error(msg)
}

func (l *Logger) Warn(msg string) {
	l.logger.Warn(msg)
}

func (l *Logger) Panic(data any) {
	l.logger.Panic(fmt.Sprint(data))
}

func (l *Logger) Fatal(msg string) {
	l.logger.Fatal(msg)
}

func (l *Logger) Sync() error {
	return l.logger.Sync()
}

func (l *Logger) OnDefer(pkg string, err error, panicData any, info string) {
	if err != nil {
		l.Error(err.Error())
	}
	if panicData != nil {
		l.Error(fmt.Sprintf("%s: raised panic, %v", pkg, panicData))
	}
	l.Info(info)
	l.Sync()
}
