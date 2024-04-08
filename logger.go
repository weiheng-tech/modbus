package modbus

type Logger interface {
	Printf(format string, args ...interface{})
}
