// Copyright 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license. See the LICENSE file for details.

package modbus

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

const (
	tcpProtocolIdentifier uint16 = 0x0000

	// Modbus Application Protocol
	tcpHeaderSize = 7
	tcpMaxLength  = 260
)

// TCPClientHandler implements Packager and Transporter interface.
type TCPClientHandler struct {
	tcpPackager
	tcpTransporter
}

// NewTCPClientHandler allocates a new TCPClientHandler.
func NewTCPClientHandler(address string) *TCPClientHandler {
	h := &TCPClientHandler{}
	h.Address = address
	h.Timeout = tcpTimeout
	h.IdleTimeout = tcpIdleTimeout
	return h
}

// tcpPackager implements Packager interface.
type tcpPackager struct {
	basePackager
	// For synchronization between messages of server & client
	transactionId uint32
	// Broadcast address is 0
	SlaveId byte
}

// Encode adds modbus application protocol header:
//
//	Transaction identifier: 2 bytes
//	Protocol identifier: 2 bytes
//	Length: 2 bytes
//	Unit identifier: 1 byte
//	Function code: 1 byte
//	Data: n bytes
func (mb *tcpPackager) Encode(pdu *ProtocolDataUnit) (adu []byte, err error) {
	adu = make([]byte, tcpHeaderSize+1+len(pdu.Data))

	// Transaction identifier
	transactionId := atomic.AddUint32(&mb.transactionId, 1)
	binary.BigEndian.PutUint16(adu, uint16(transactionId))
	// Protocol identifier
	binary.BigEndian.PutUint16(adu[2:], tcpProtocolIdentifier)
	// Length = sizeof(SlaveId) + sizeof(FunctionCode) + Data
	length := uint16(1 + 1 + len(pdu.Data))
	binary.BigEndian.PutUint16(adu[4:], length)
	// Unit identifier
	adu[6] = mb.SlaveId

	// PDU
	adu[tcpHeaderSize] = pdu.FunctionCode
	copy(adu[tcpHeaderSize+1:], pdu.Data)
	return
}

// Verify confirms transaction, protocol and unit id.
func (mb *tcpPackager) Verify(aduRequest []byte, aduResponse []byte) (err error) {
	// Transaction id
	responseVal := binary.BigEndian.Uint16(aduResponse)
	requestVal := binary.BigEndian.Uint16(aduRequest)
	if responseVal != requestVal {
		err = fmt.Errorf("modbus: response transaction id '%v' does not match request '%v'", responseVal, requestVal)
		return
	}
	// Protocol id
	responseVal = binary.BigEndian.Uint16(aduResponse[2:])
	requestVal = binary.BigEndian.Uint16(aduRequest[2:])
	if responseVal != requestVal {
		err = fmt.Errorf("modbus: response protocol id '%v' does not match request '%v'", responseVal, requestVal)
		return
	}
	// Unit id (1 byte)
	if aduResponse[6] != aduRequest[6] {
		err = fmt.Errorf("modbus: response unit id '%v' does not match request '%v'", aduResponse[6], aduRequest[6])
		return
	}
	return
}

// Decode extracts PDU from TCP frame:
//
//	Transaction identifier: 2 bytes
//	Protocol identifier: 2 bytes
//	Length: 2 bytes
//	Unit identifier: 1 byte
func (mb *tcpPackager) Decode(adu []byte) (pdu *ProtocolDataUnit, err error) {
	// Read length value in the header
	length := binary.BigEndian.Uint16(adu[4:])
	pduLength := len(adu) - tcpHeaderSize
	if pduLength <= 0 || pduLength != int(length-1) {
		err = fmt.Errorf("modbus: length in response '%v' does not match pdu data length '%v'", length-1, pduLength)
		return
	}
	pdu = &ProtocolDataUnit{}
	// The first byte after header is function code
	pdu.FunctionCode = adu[tcpHeaderSize]
	pdu.Data = adu[tcpHeaderSize+1:]
	return
}

// tcpTransporter implements Transporter interface.
type tcpTransporter struct {
	TcpTransporter
}

// Send sends data to server and ensures response length is greater than header length.
func (mb *tcpTransporter) Send(aduRequest []byte) (aduResponse []byte, err error) {
	mb.Mu.Lock()
	defer mb.Mu.Unlock()

	// Establish a new connection if not connected
	if err = mb.Connect(); err != nil {
		return
	}
	// Set timer to close when idle
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
	// Send data
	mb.Logf("modbus: sending % x", aduRequest)
	if _, err = mb.Conn.Write(aduRequest); err != nil {
		_ = mb.ConnClose()
		return
	}
	// Read header first
	var data [tcpMaxLength]byte
	if _, err = io.ReadFull(mb.Conn, data[:tcpHeaderSize]); err != nil {
		return
	}
	// Read length, ignore transaction & protocol id (4 bytes)
	length := int(binary.BigEndian.Uint16(data[4:]))
	if length <= 0 {
		mb.Flush(data[:])
		err = fmt.Errorf("modbus: length in response header '%v' must not be zero", length)
		return
	}
	if length > (tcpMaxLength - (tcpHeaderSize - 1)) {
		mb.Flush(data[:])
		err = fmt.Errorf("modbus: length in response header '%v' must not greater than '%v'", length, tcpMaxLength-tcpHeaderSize+1)
		return
	}
	// Skip unit id
	length += tcpHeaderSize - 1
	if _, err = io.ReadFull(mb.Conn, data[tcpHeaderSize:length]); err != nil {
		return
	}
	aduResponse = data[:length]
	mb.Logf("modbus: received % x\n", aduResponse)
	return
}
