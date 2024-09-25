// Copyright 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license. See the LICENSE file for details.

package modbus

import (
	"net"
	"sync"
	"time"
)

const (
	// Default TCP timeout is not set
	tcpTimeout     = 10 * time.Second
	tcpIdleTimeout = 60 * time.Second
)

func NewTcpPort(address string, timeout, idleTimeout time.Duration) *TcpPort {
	tp := &TcpPort{
		Address:     address,
		Timeout:     timeout,
		IdleTimeout: idleTimeout,
	}
	if tp.Timeout == 0 {
		tp.Timeout = tcpTimeout
	}
	if tp.IdleTimeout == 0 {
		tp.IdleTimeout = tcpIdleTimeout
	}

	return tp
}

// TcpPort implements Transporter interface.
type TcpPort struct {
	// Connect string
	Address string
	// Connect & Read timeout
	Timeout time.Duration
	// Idle timeout to close the connection
	IdleTimeout time.Duration
	// Transmission logger
	Logger Logger

	// TCP connection
	Mu           sync.Mutex
	Conn         net.Conn
	closeTimer   *time.Timer
	LastActivity time.Time
}

func (mb *TcpPort) Connect() error {
	if mb.Conn == nil {
		dialer := net.Dialer{Timeout: mb.Timeout}
		conn, err := dialer.Dial("tcp", mb.Address)
		if err != nil {
			return err
		}
		mb.Conn = conn
	}
	return nil
}

// Close closes current connection.
func (mb *TcpPort) Close() error {
	mb.Mu.Lock()
	defer mb.Mu.Unlock()

	return mb.ConnClose()
}

// ConnClose closeLocked closes current connection. Caller must hold the mutex before calling this method.
func (mb *TcpPort) ConnClose() (err error) {
	if mb.Conn != nil {
		err = mb.Conn.Close()
		mb.Conn = nil
	}
	return
}

func (mb *TcpPort) StartCloseTimer() {
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
func (mb *TcpPort) closeIdle() {
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

// Flush flushes pending data in the connection,
// returns io.EOF if connection is closed.
func (mb *TcpPort) Flush(b []byte) (err error) {
	if err = mb.Conn.SetReadDeadline(time.Now()); err != nil {
		return
	}
	// Timeout setting will be reset when reading
	if _, err = mb.Conn.Read(b); err != nil {
		// Ignore timeout error
		if netError, ok := err.(net.Error); ok && netError.Timeout() {
			err = nil
		}
	}
	return
}

func (mb *TcpPort) Debugf(format string, v ...interface{}) {
	if mb.Logger != nil {
		mb.Logger.Debugf(format, v...)
	}
}
