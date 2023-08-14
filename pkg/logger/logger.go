package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Logger interface {
	With(key string, value any) Logger
	Info(msg string)
	Error(msg string)
	Panic(data any)
	Fatal(msg string)
	Sync() error
}

type logger struct {
	logger *zap.Logger
}

var ChatLogger *logger = new(logger)

func init() {
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(config)
	writer := zapcore.AddSync(&lumberjack.Logger{
		//needs config file
		Filename:   "../../logs/server.log",
		MaxSize:    100,
		MaxBackups: 1,
		MaxAge:     1,
	})
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, writer, zapcore.InfoLevel),
	)

	ChatLogger.logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
}

func (l *logger) With(key string, data any) Logger {
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
	return &logger{logger: l.logger.With(field)}
}

func (l *logger) WithEventField(event string) Logger {
	return l.With("event", event)
}

func (l *logger) Info(msg string) {
	l.logger.Info(msg)
}

func (l *logger) Error(msg string) {
	l.logger.Error(msg)
}

func (l *logger) Panic(data any) {
	l.logger.Panic(fmt.Sprint(data))
}

func (l *logger) Fatal(msg string) {
	l.logger.Fatal(msg)
}

func (l *logger) Sync() error {
	return l.logger.Sync()
}
