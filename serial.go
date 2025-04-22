// Copyright 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license. See the LICENSE file for details.

package modbus

import (
	"fmt"
	"io"
	"sync"
	"time"

	"go.bug.st/serial"
)

const (
	// Default timeout
	serialTimeout     = 5 * time.Second
	serialIdleTimeout = 60 * time.Second
)

type Config struct {
	// Device path (/dev/ttyS0)
	Address string
	// Baud rate (default 19200)
	BaudRate int
	// Data bits: 5, 6, 7 or 8 (default 8)
	DataBits int
	// Stop bits: 1 or 2 (default 1)
	StopBits int
	// Parity: N - None, E - Even, O - Odd (default E)
	// (The use of no parity requires 2 stop bits.)
	Parity string
	// Read (Write) timeout.
	Timeout time.Duration
	// Configuration related to RS485
	RS485 RS485Config
}

type RS485Config struct {
	// Enable RS485 support
	Enabled bool
	// Use RS485 Alternative Operation, directly handle RTS pin via ioctl
	RS485Alternative bool
	// Delay RTS prior to send
	DelayRtsBeforeSend time.Duration
	// Delay RTS after send
	DelayRtsAfterSend time.Duration
	// Set RTS high during send
	RtsHighDuringSend bool
	// Set RTS high after send
	RtsHighAfterSend bool
	// Rx during Tx
	RxDuringTx bool
}

func NewSerialPort(c Config, idleTimeout time.Duration) *SerialPort {
	if c.Timeout == 0 {
		c.Timeout = serialTimeout
	}

	sp := &SerialPort{
		Config:      c,
		IdleTimeout: idleTimeout,
	}
	if sp.IdleTimeout == 0 {
		sp.IdleTimeout = serialIdleTimeout
	}
	return sp
}

// SerialPort has configuration and I/O controller.
type SerialPort struct {
	// Serial port configuration.
	Config
	IdleTimeout        time.Duration
	QueryDelayDuration time.Duration // Query delay duration
	Logger             Logger

	Mu           sync.Mutex
	Conn         io.ReadWriteCloser // port is platform-dependent data structure for serial port.
	closeTimer   *time.Timer
	LastActivity time.Time
}

// Connect connects to the serial port if it is not connected. Caller must hold the mutex.
func (mb *SerialPort) Connect() error {
	var parity serial.Parity
	switch mb.Parity {
	case "N":
		parity = serial.NoParity
	case "E":
		parity = serial.EvenParity
	case "O":
		parity = serial.OddParity
	default:
		return fmt.Errorf("invalid parity: %s", mb.Parity)
	}

	if mb.Conn == nil {
		port, err := serial.Open(mb.Config.Address, &serial.Mode{
			BaudRate: mb.BaudRate,
			DataBits: mb.DataBits,
			StopBits: serial.StopBits(mb.StopBits),
			Parity:   parity,
		})
		if err != nil {
			return err
		}

		err = port.SetReadTimeout(mb.Config.Timeout)
		if err != nil {
			return err
		}
		mb.Conn = port
	}
	return nil
}

func (mb *SerialPort) Close() (err error) {
	mb.Mu.Lock()
	defer mb.Mu.Unlock()

	return mb.ConnClose()
}

// ConnClose closes the serial port if it is connected. Caller must hold the mutex.
func (mb *SerialPort) ConnClose() (err error) {
	if mb.Conn != nil {
		err = mb.Conn.Close()
		mb.Conn = nil
	}
	return
}

func (mb *SerialPort) Debugf(format string, v ...interface{}) {
	if mb.Logger != nil {
		mb.Logger.Debugf(format, v...)
	}
}

func (mb *SerialPort) StartCloseTimer() {
	if mb.IdleTimeout <= 0 {
		return
	}
	if mb.closeTimer == nil {
		mb.closeTimer = time.AfterFunc(mb.IdleTimeout, mb.closeIdle)
	} else {
		mb.closeTimer.Reset(mb.IdleTimeout)
	}
}

// closeIdle closes the connection if last activity is passed behind IdleTimeout.
func (mb *SerialPort) closeIdle() {
	mb.Mu.Lock()
	defer mb.Mu.Unlock()

	if mb.IdleTimeout <= 0 {
		return
	}
	idle := time.Now().Sub(mb.LastActivity)
	if idle >= mb.IdleTimeout {
		mb.Debugf("modbus: closing connection due to idle timeout: %v", idle)
		mb.ConnClose()
	}
}
