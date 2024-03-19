package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
)

type PacketHeader struct {
	PacketFormat           uint16  // 2023
	GameYear               uint8   // Game year - last two digits e.g. 23
	GameMajorVersion       uint8   // Game major version - "X.00"
	GameMinorVersion       uint8   // Game minor version - "1.XX"
	PacketVersion          uint8   // Version of this packet type, all start from 1
	PacketId               uint8   // Identifier for the packet type, see below
	SessionUID             uint64  // Unique identifier for the session
	SessionTime            float32 // Session timestamp
	FrameIdentifier        uint32  // Identifier for the frame the data was retrieved on
	OverallFrameIdentifier uint32  // Overall identifier for the frame the data was retrieved
	// on, doesn't go back after flashbacks
	PlayerCarIndex          uint8 // Index of player's car in the array
	SecondaryPlayerCarIndex uint8 // Index of secondary player's car in the array (splitscreen)
	// 255 if no second player
}

type EventData struct {
	PacketHeader
	EventStringCode [4]uint8
}

func main() {
	var listener net.ListenConfig
	conn, err := listener.ListenPacket(context.TODO(), "udp4", "0.0.0.0:27543")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer conn.Close()
	// Read from UDP listener in endless loop
	println(conn.LocalAddr().String())

	// the largest packet is just 1460 bytes
	var buff []byte = make([]byte, 1460)
	for {
		numRead, _, err := conn.ReadFrom(buff)
		if numRead > 0 {
			println(fmt.Sprintf("read %d bytes", numRead))
			doSomethingWithAPacket(buff[:numRead])
			continue
		}

		if err != nil {
			panic(err)
		}
	}

	
}

func doSomethingWithAPacket(packet []byte) {
	var header EventData
	buf := bytes.NewBuffer(packet)
	binary.Read(buf, binary.LittleEndian, &header)
	// os.WriteFile("foo.out", packet, fs.ModeAppend)
	// println("read packet")

	fmt.Printf("%+v\n%s\n", header, header.EventStringCode)
}
