// Copyright 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license. See the LICENSE file for details.

package modbus

import (
	"encoding/binary"
	"fmt"
)

// ENRtuProtocolHandler implements Packager and Transporter interface.
type ENRtuProtocolHandler struct {
	enRtuPackager
	SerialPort
}

// NewENRtuProtocolHandler allocates and initializes a ENRtuProtocolHandler.
func NewENRtuProtocolHandler(address string, baudRate int) *ENRtuProtocolHandler {
	handler := &ENRtuProtocolHandler{}
	handler.Address = address
	handler.Timeout = serialTimeout
	handler.IdleTimeout = serialIdleTimeout
	handler.BaudRate = baudRate
	return handler
}

type ENRtuOverTCPProtocolHandler struct {
	enRtuPackager
	rtuOverTcpTransporter
}

// NewENRtuOverTCPProtocolHandler allocates and initializes a ENRtuOverTCPProtocolHandler.
func NewENRtuOverTCPProtocolHandler(address string, baudRate int) *ENRtuOverTCPProtocolHandler {
	handler := &ENRtuOverTCPProtocolHandler{}
	handler.Address = address
	handler.Timeout = tcpTimeout
	handler.IdleTimeout = tcpIdleTimeout
	handler.BaudRate = baudRate
	return handler
}

// rtuPackager implements Packager interface.
type enRtuPackager struct {
	SlaveId byte
	GunId   byte
}

// dataBlock creates a sequence of uint16 data.
func (mb *enRtuPackager) dataBlock(value ...uint16) []byte {
	data := make([]byte, 2*len(value)+1)
	for i, v := range value {
		binary.BigEndian.PutUint16(data[i*2:], v)
	}
	data[len(data)-1] = mb.GunId
	return data
}

// dataBlockSuffix creates a sequence of uint16 data and append the suffix plus its length.
func (mb *enRtuPackager) dataBlockSuffix(suffix []byte, value ...uint16) []byte {
	length := 2 * len(value)
	data := make([]byte, length+2+len(suffix))
	for i, v := range value {
		binary.BigEndian.PutUint16(data[i*2:], v)
	}
	data[length] = mb.GunId
	data[length+1] = uint8(len(suffix))
	copy(data[length+2:], suffix)
	return data
}

// Encode encodes PDU in a RTU frame:
//
//	Slave Address   : 1 byte
//	Function        : 1 byte
//	Data            : 0 up to 252 bytes
//	CRC             : 2 byte
func (mb *enRtuPackager) Encode(pdu *ProtocolDataUnit) (adu []byte, err error) {
	length := len(pdu.Data) + 4
	if length > rtuMaxSize {
		err = fmt.Errorf("modbus: length of data '%v' must not be bigger than '%v'", length, rtuMaxSize)
		return
	}
	adu = make([]byte, length)

	adu[0] = mb.SlaveId
	adu[1] = pdu.FunctionCode
	copy(adu[2:], pdu.Data)

	// Append crc
	var crc crc
	crc.reset().pushBytes(adu[0 : length-2])
	checksum := crc.value()

	adu[length-1] = byte(checksum >> 8)
	adu[length-2] = byte(checksum)
	return
}

// Verify verifies response length and slave id.
func (mb *enRtuPackager) Verify(aduRequest []byte, aduResponse []byte) (err error) {
	length := len(aduResponse)
	// Minimum size (including address, function and CRC)
	if length < rtuMinSize {
		err = fmt.Errorf("modbus: response length '%v' does not meet minimum '%v'", length, rtuMinSize)
		return
	}
	// Slave address must match
	if aduResponse[0] != aduRequest[0] {
		err = fmt.Errorf("modbus: response slave id '%v' does not match request '%v'", aduResponse[0], aduRequest[0])
		return
	}
	return
}

// Decode extracts PDU from RTU frame and verify CRC.
func (mb *enRtuPackager) Decode(adu []byte) (pdu *ProtocolDataUnit, err error) {
	length := len(adu)
	// Calculate checksum
	var crc crc
	crc.reset().pushBytes(adu[0 : length-2])
	checksum := uint16(adu[length-1])<<8 | uint16(adu[length-2])
	if checksum != crc.value() {
		err = fmt.Errorf("modbus: response crc '%v' does not match expected '%v'", checksum, crc.value())
		return
	}
	// Function code & data
	pdu = &ProtocolDataUnit{}
	pdu.FunctionCode = adu[1]
	pdu.Data = adu[2 : length-2]

	// TODO
	switch pdu.FunctionCode {
	case FuncCodeReadCoils, FuncCodeReadInputRegisters, FuncCodeReadHoldingRegisters:
		pdu.Data = removeElement(pdu.Data, 1)
	case FuncCodeWriteSingleRegister:
		pdu.Data = removeElement(pdu.Data, 2)
	case FuncCodeWriteMultipleRegisters:
		pdu.Data = removeElement(pdu.Data, 4)
	default:
		err = fmt.Errorf("modbus: en+ unsupported function code in response '%v'", pdu.FunctionCode)
	}

	return
}

func removeElement[T any](slice []T, i int) []T {
	slice[i] = slice[len(slice)-1]
	return slice[:len(slice)-1]
}
