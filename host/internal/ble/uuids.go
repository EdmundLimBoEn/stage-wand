package ble

// GATT UUIDs copied from Shared/Bluetooth.swift. Payloads are the same JSON
// frames as the LAN WebSocket. One ATT write or notification carries one
// complete JSON object. The default 23-byte ATT MTU is too small for auth, so
// the central must exchange a larger MTU before the first command. Apple does
// that during the connection. Android calls requestMtu(517). BlueZ and WinRT
// accept the central's MTU request. There is no extra BLE framing layer.
const (
	ServiceUUID = "5A3E0001-8B6C-4B1E-9F8D-2C7A1D4E6F01"
	CommandUUID = "5A3E0002-8B6C-4B1E-9F8D-2C7A1D4E6F01"
	ReplyUUID   = "5A3E0003-8B6C-4B1E-9F8D-2C7A1D4E6F01"
)
