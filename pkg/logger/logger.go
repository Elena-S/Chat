package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Logger *zap.Logger

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

	Logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

}

func EventField(event string) zapcore.Field {
	return zapcore.Field{
		Key:    "event",
		Type:   zapcore.StringType,
		String: event,
	}
}

func ErrorPanic(ctxLogger *zap.Logger, data any) {
	ctxLogger.Error("raised panic", zapcore.Field{
		Key:    "data",
		Type:   zapcore.StringType,
		String: fmt.Sprint(data),
	})
}

type loggerWS struct {
	fieldUserID  zapcore.Field
	fieldConnNum zapcore.Field
	fieldMessage zapcore.Field
	logger       *zap.Logger
}

func NewLoggerWS(ctxLogger *zap.Logger, remoteAddr string) *loggerWS {
	l := new(loggerWS)

	l.fieldUserID = zapcore.Field{
		Key:  "user id",
		Type: zapcore.Uint64Type,
	}
	l.fieldConnNum = zapcore.Field{
		Key:  "connection number",
		Type: zapcore.Uint64Type,
	}
	l.fieldMessage = zapcore.Field{
		Key:       "message",
		Type:      zapcore.ByteStringType,
		Interface: []byte{},
	}
	l.logger = ctxLogger.With(
		EventField("ws connection"),
		zapcore.Field{
			Key:    "remote addr",
			Type:   zapcore.StringType,
			String: remoteAddr,
		},
	)

	return l
}

func (l *loggerWS) SetUserID(userID uint) {
	l.fieldUserID.Integer = int64(userID)
}

func (l *loggerWS) SetNumConn(numConn uint64) {
	l.fieldConnNum.Integer = int64(numConn)
}

func (l *loggerWS) SetMessage(msg []byte) {
	l.fieldMessage.Interface = msg
}

func (l *loggerWS) Info(msg string) {
	l.logger.Info(msg, l.fieldUserID, l.fieldConnNum, l.fieldMessage)
}

func (l *loggerWS) Error(msg string) {
	l.logger.Error(msg, l.fieldUserID, l.fieldConnNum, l.fieldMessage)
}
