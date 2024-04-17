// Copyright 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license. See the LICENSE file for details.

package modbus

import (
	"io"
	"sync"
	"time"

	"github.com/jifanchn/serial"
)

const (
	// Default timeout
	serialTimeout     = 5 * time.Second
	serialIdleTimeout = 60 * time.Second
)

func NewSerialPort(c serial.Config, idleTimeout time.Duration) *SerialPort {
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
	serial.Config
	IdleTimeout time.Duration
	Logger      Logger

	Mu           sync.Mutex
	Conn         io.ReadWriteCloser // port is platform-dependent data structure for serial port.
	closeTimer   *time.Timer
	LastActivity time.Time
}

// Connect connects to the serial port if it is not connected. Caller must hold the mutex.
func (mb *SerialPort) Connect() error {
	if mb.Conn == nil {
		port, err := serial.Open(&mb.Config)
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
		if !mb.closeTimer.Stop() {
			<-mb.closeTimer.C
		}
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
