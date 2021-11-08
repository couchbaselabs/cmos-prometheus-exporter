package config

import (
	"github.com/couchbase/goutils/logging"
	"go.uber.org/zap"
)

type GoUtilsZapLogger struct {
	Logger *zap.SugaredLogger
}

func (g *GoUtilsZapLogger) Loga(level logging.Level, f func() string) {
	switch level {
	case logging.TRACE:
		g.Tracea(f)
	case logging.DEBUG:
		g.Debuga(f)
	case logging.INFO:
		g.Infoa(f)
	case logging.WARN:
		g.Warna(f)
	case logging.ERROR:
		g.Errora(f)
	case logging.SEVERE:
		g.Severea(f)
	case logging.FATAL:
		g.Fatala(f)
	}
}

func (g *GoUtilsZapLogger) Debuga(f func() string) {
	g.Debugf(f())
}

func (g *GoUtilsZapLogger) Tracea(f func() string) {
	g.Tracef(f())
}

func (g *GoUtilsZapLogger) Requesta(rlevel logging.Level, f func() string) {
	g.Requestf(rlevel, f())
}

func (g *GoUtilsZapLogger) Infoa(f func() string) {
	g.Infof(f())
}

func (g *GoUtilsZapLogger) Warna(f func() string) {
	g.Warnf(f())
}

func (g *GoUtilsZapLogger) Errora(f func() string) {
	g.Errorf(f())
}

func (g *GoUtilsZapLogger) Severea(f func() string) {
	g.Severef(f())
}

func (g *GoUtilsZapLogger) Fatala(f func() string) {
	g.Fatalf(f())
}

func (g *GoUtilsZapLogger) Logf(level logging.Level, fmt string, args ...interface{}) {
	switch level {
	case logging.TRACE:
		g.Tracef(fmt, args...)
	case logging.DEBUG:
		g.Debugf(fmt, args...)
	case logging.INFO:
		g.Infof(fmt, args...)
	case logging.WARN:
		g.Warnf(fmt, args...)
	case logging.ERROR:
		g.Errorf(fmt, args...)
	case logging.SEVERE:
		g.Severef(fmt, args...)
	case logging.FATAL:
		g.Fatalf(fmt, args...)
	}
}

func (g *GoUtilsZapLogger) Debugf(fmt string, args ...interface{}) {
	g.Logger.Debugf(fmt, args...)
}

func (g *GoUtilsZapLogger) Tracef(fmt string, args ...interface{}) {
	g.Logger.Infof(fmt, args...)
}

func (g *GoUtilsZapLogger) Requestf(rlevel logging.Level, fmt string, args ...interface{}) {
	g.Logf(rlevel, fmt, args...)
}

func (g *GoUtilsZapLogger) Infof(fmt string, args ...interface{}) {
	g.Logger.Infof(fmt, args...)
}

func (g *GoUtilsZapLogger) Warnf(fmt string, args ...interface{}) {
	g.Logger.Warnf(fmt, args...)
}

func (g *GoUtilsZapLogger) Errorf(fmt string, args ...interface{}) {
	g.Logger.Errorf(fmt, args...)
}

func (g *GoUtilsZapLogger) Severef(fmt string, args ...interface{}) {
	g.Logger.Errorf(fmt, args...)
}

func (g *GoUtilsZapLogger) Fatalf(fmt string, args ...interface{}) {
	g.Logger.Panicf(fmt, args...)
}

func (g *GoUtilsZapLogger) SetLevel(level logging.Level) {
	panic("unsupported")
}

func (g *GoUtilsZapLogger) Level() logging.Level {
	// Zap doesn't have a simple notion of "the current level", so return the most verbose possible
	// and filter out at the Zap level
	return logging.TRACE
}
