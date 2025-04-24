// Copyright 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license. See the LICENSE file for details.

package modbus

import (
	"io"
	"time"
)

// RTUOverTcpClientHandler implements Packager and Transporter interface.
type RTUOverTcpClientHandler struct {
	RtuPackager
	rtuOverTcpTransporter
}

// NewRTUOverTcpClientHandler allocates and initializes a RTUOverTcpClientHandler.
func NewRTUOverTcpClientHandler(address string) *RTUOverTcpClientHandler {
	handler := &RTUOverTcpClientHandler{}
	handler.Address = address
	handler.Timeout = tcpTimeout
	handler.IdleTimeout = tcpIdleTimeout
	return handler
}

// rtuSerialTransporter implements Transporter interface.
type rtuOverTcpTransporter struct {
	TcpPort
	BaudRate int
}

func (mb *rtuOverTcpTransporter) Send(aduRequest []byte) (aduResponse []byte, err error) {
	mb.Mu.Lock()
	defer func() {
		if mb.QueryDelayDuration > 0 {
			time.Sleep(mb.QueryDelayDuration)
		}
		mb.Mu.Unlock()
	}()

	// Make sure port is connected
	if err = mb.Connect(); err != nil {
		return
	}
	// Start the timer to close when idle
	mb.LastActivity = time.Now()
	mb.StartCloseTimer()

	// Set write and read timeout
	var timeout time.Time
	if mb.Timeout > 0 {
		timeout = mb.LastActivity.Add(mb.Timeout)
	}
	if err = mb.Conn.SetDeadline(timeout); err != nil {
		_ = mb.ConnClose()
		return
	}

	// Send the request
	mb.Debugf("modbus: sending % x", aduRequest)
	if _, err = mb.Conn.Write(aduRequest); err != nil {
		_ = mb.ConnClose()
		return
	}
	function := aduRequest[1]
	functionFail := aduRequest[1] & 0x80
	bytesToRead := calculateResponseLength(aduRequest)
	time.Sleep(mb.calculateDelay(len(aduRequest) + bytesToRead))

	var n int
	var n1 int
	var data [rtuMaxSize]byte
	//We first read the minimum length and then read either the full package
	//or the error package, depending on the error status (byte 2 of the response)
	n, err = io.ReadAtLeast(mb.Conn, data[:], rtuMinSize)
	if err != nil {
		return
	}
	//if the function is correct
	if data[1] == function {
		//we read the rest of the bytes
		if n < bytesToRead {
			if bytesToRead > rtuMinSize && bytesToRead <= rtuMaxSize {
				if bytesToRead > n {
					n1, err = io.ReadFull(mb.Conn, data[n:bytesToRead])
					n += n1
				}
			}
		}
	} else if data[1] == functionFail {
		//for error, we need to read 5 bytes
		if n < rtuExceptionSize {
			n1, err = io.ReadFull(mb.Conn, data[n:rtuExceptionSize])
		}
		n += n1
	}

	if err != nil {
		return
	}
	aduResponse = data[:n]
	mb.Debugf("modbus: received % x", aduResponse)
	return
}

// calculateDelay roughly calculates time needed for the next frame.
// See MODBUS over Serial Line - Specification and Implementation Guide (page 13).
func (mb *rtuOverTcpTransporter) calculateDelay(chars int) time.Duration {
	var characterDelay, frameDelay int // us

	if mb.BaudRate <= 0 || mb.BaudRate > 19200 {
		characterDelay = 750
		frameDelay = 1750
	} else {
		characterDelay = 15000000 / mb.BaudRate
		frameDelay = 35000000 / mb.BaudRate
	}
	return time.Duration(characterDelay*chars+frameDelay) * time.Microsecond
}
