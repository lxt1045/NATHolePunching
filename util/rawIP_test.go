package util

import (
	"net"
	"testing"
)

func TestSendSYNPackage(t *testing.T) {
	var (
		srcIP, dstIP     []byte
		srcPort, dstPort uint16
	)
	srcIP = net.ParseIP("192.168.137.111")
	srcIP = net.ParseIP("192.168.137.111")
	srcPort = 8080
	dstPort = 8089
	SendSYNPackage(srcIP, dstIP, srcPort, dstPort)
}
