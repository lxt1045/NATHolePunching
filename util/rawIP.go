package util

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"unsafe"
)

const (
	OFFSET_SrcIP         = 0
	OFFSET_DstIP         = 4
	OFFSET_SrcPort       = 8
	OFFSET_DstPort       = 10
	OFFSET_SynNO         = 12
	OFFSET_AckNO         = 16
	OFFSET_Code          = 20
	OFFSET_Window        = 22
	OFFSET_Checksum      = 24
	OFFSET_UrgentPointer = 26
)

func getHearderMember(base unsafe.Pointer, offset int) unsafe.Pointer {
	//pb := (*int16)(unsafe.Pointer(uintptr(unsafe.Pointer(&x)) + unsafe.Offsetof(x.b)))
	return (unsafe.Pointer)(unsafe.Pointer(uintptr(base) + uintptr(offset)))
}
func setHearderMember(base unsafe.Pointer, offset int, value uint32, mask uint32) {
	//pb := (*int16)(unsafe.Pointer(uintptr(unsafe.Pointer(&x)) + unsafe.Offsetof(x.b)))
	p := (*uint32)(unsafe.Pointer(uintptr(base) + uintptr(offset)))
	v := (*p) & (^mask)
	*p = v | (value & mask)
}
func makeTCPPackage0(SrcIP, DstIP uint32, SrcPort, DstPort uint16, SynNO, AckNO uint32, SYN, ACK bool) []byte {
	var baseArray [40]byte //最大值是 0xf*4byte = 64byte
	basePtr := (unsafe.Pointer)(&baseArray)
	setHearderMember(basePtr, OFFSET_SrcIP, SrcIP, 0xffffffff)
	setHearderMember(basePtr, OFFSET_DstIP, DstIP, 0xffffffff)
	setHearderMember(basePtr, OFFSET_SrcPort, uint32(SrcPort), 0xffff)
	setHearderMember(basePtr, OFFSET_DstPort, uint32(DstPort), 0xffff)
	setHearderMember(basePtr, OFFSET_SynNO, SynNO, 0xffffffff)
	setHearderMember(basePtr, OFFSET_AckNO, AckNO, 0xffffffff)

	//Code: DataOffset--4bit,Reserved--6bit,URG,ACK,PSH,RST,SYN,FIN各1bit
	dataOffset := uint16(5) //一共5*4byte == 20byte
	Code := dataOffset
	if SYN {
		Code |= (1 << 11)
	}
	if ACK {
		Code |= (1 << 14)
	}
	setHearderMember(basePtr, OFFSET_Code, uint32(Code), 0xffff)

	setHearderMember(basePtr, OFFSET_Window, 1024, 0xffff)
	setHearderMember(basePtr, OFFSET_Checksum, 0, 0xffff)
	setHearderMember(basePtr, OFFSET_UrgentPointer, 0, 0xffff)

	checksum := CheckSum(baseArray[:20])
	setHearderMember(basePtr, OFFSET_Checksum, uint32(checksum), 0xffff)

	return baseArray[8:20] //不需要IP地址
}
func SendSYNPackage0(conn *net.IPConn) []byte {
	ip2Uint32 := func(ip net.IP) (rIP uint32) {
		for i := 0; i < 4; i++ {
			rIP = (rIP << 8) | uint32(ip[i])
		}
		return
	}
	c := *conn
	src := strings.Split(c.LocalAddr().Network(), ":")
	dst := strings.Split(c.RemoteAddr().Network(), ":")
	srcIP := net.ParseIP(src[0])
	dstIP := net.ParseIP(dst[0])
	srcPort, e1 := strconv.ParseUint(src[1], 10, 32)
	dstPort, e2 := strconv.ParseUint(dst[1], 10, 32)
	if e1 != nil || e2 != nil {
		Mylog.Error("err")
	}
	buf := makeTCPPackage0(ip2Uint32(srcIP), ip2Uint32(dstIP), uint16(srcPort), uint16(dstPort), 0, 0, true, false)
	return buf
}

type TCP struct {
	SrcIP, DstIP     uint32
	SrcPort, DstPort uint16
	SynNO, AckNO     uint32
	//Code: DataOffset--4bit,Reserved--6bit,URG,ACK,PSH,RST,SYN,FIN各1bit
	Code, Window, Checksum, UrgentPointer uint16
}
type ICMP struct {
	Type        uint8
	Code        uint8
	Checksum    uint16
	Identifier  uint16
	SequenceNum uint16
}

func CheckSum(data []byte) uint16 {
	/*
		首先，把伪首部、TCP报头、TCP数据分为16位的字，如果总长度为奇数个字节，则在最后增添一个位都为0的字节。
		            把TCP报头中的校验和字段置为0（否则就陷入鸡生蛋还是蛋生鸡的问题）。
		其次，用反码相加法累加所有的16位字（进位也要累加,累加多少次？）。
		最后，对计算结果取反，作为TCP的校验和。
	*/
	var (
		sum    uint32
		length int = len(data)
		index  int
	)
	for length > 1 {
		sum += uint32(data[index])<<8 + uint32(data[index+1])
		index += 2
		length -= 2
	}
	if length > 0 {
		sum += uint32(data[index])
	}
	// sum = (sum >> 16) + (sum & 0xffff);
	// sum += (sum >> 16);
	sum += (sum >> 16)

	return uint16(^sum)
}

func makeTCPPackage(srcIP, dstIP []byte, srcPort, dstPort uint16, SynNO, AckNO uint32, SYN, ACK bool) (bufRet []byte) {
	ip2Uint32 := func(ip []byte) (rIP uint32) {
		for i := 0; i < 4; i++ {
			rIP = (rIP << 8) | uint32(ip[i])
		}
		return
	}

	//Code: DataOffset--4bit,Reserved--6bit,URG,ACK,PSH,RST,SYN,FIN各1bit
	dataOffset := uint16(5) //一共5*4byte == 20byte
	Code := dataOffset
	if SYN {
		Code |= (1 << 11)
	}
	if ACK {
		Code |= (1 << 14)
	}
	tcpHeader := TCP{
		SrcIP:         ip2Uint32(srcIP),
		DstIP:         ip2Uint32(dstIP),
		SrcPort:       uint16(srcPort),
		DstPort:       uint16(dstPort),
		SynNO:         0,
		AckNO:         0,
		Code:          Code, //Code: DataOffset--4bit,Reserved--6bit,URG,ACK,PSH,RST,SYN,FIN各1bit
		Window:        1024,
		Checksum:      0,
		UrgentPointer: 0,
	}
	var buffer bytes.Buffer
	//先在buffer中写入icmp数据报求去校验和
	binary.Write(&buffer, binary.BigEndian, tcpHeader)
	tcpHeader.Checksum = CheckSum(buffer.Bytes())
	//然后清空buffer并把求完校验和的icmp数据报写入其中准备发送
	buffer.Reset()
	binary.Write(&buffer, binary.BigEndian, tcpHeader)

	fmt.Printf("send icmp packet success!")
	return
}

func SendSYNPackage(srcIP, dstIP []byte, srcPort, dstPort uint16) error {
	Mylog.Debugf("srcIP:%v, dstIP:%v, srcPort:%d, dstPort:%d", srcIP, dstIP, srcPort, dstPort)

	laddr := net.IPAddr{IP: srcIP} //***IP地址改成你自己的网段***
	raddr := net.IPAddr{IP: dstIP}
	//如果你要使用网络层的其他协议还可以设置成 ip:ospf、ip:arp 等
	conn, err := net.DialIP("ip4:tcp", &laddr, &raddr)
	if err != nil {
		Mylog.Error(err.Error())
		return err
	}
	defer conn.Close()

	buf := makeTCPPackage(srcIP, dstIP, srcPort, dstPort, 0, 0, true, false)

	if _, err := conn.Write(buf); err != nil {
		fmt.Println(err.Error())
		return err
	}
	Mylog.Debug("send icmp packet success!\n", buf)
	return nil
}

func main() {
	var (
		icmp  ICMP
		laddr net.IPAddr = net.IPAddr{IP: net.ParseIP("192.168.137.111")} //***IP地址改成你自己的网段***
		raddr net.IPAddr = net.IPAddr{IP: net.ParseIP("192.168.137.1")}
	)
	//如果你要使用网络层的其他协议还可以设置成 ip:ospf、ip:arp 等
	conn, err := net.DialIP("ip4:tcp", &laddr, &raddr)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	defer conn.Close()

	//开始填充数据包
	icmp.Type = 8 //8->echo message  0->reply message
	icmp.Code = 0
	icmp.Checksum = 0
	icmp.Identifier = 0
	icmp.SequenceNum = 0

	var (
		buffer bytes.Buffer
	)
	//先在buffer中写入icmp数据报求去校验和
	binary.Write(&buffer, binary.BigEndian, icmp)
	icmp.Checksum = CheckSum(buffer.Bytes())
	//然后清空buffer并把求完校验和的icmp数据报写入其中准备发送
	buffer.Reset()
	binary.Write(&buffer, binary.BigEndian, icmp)

	if _, err := conn.Write(buffer.Bytes()); err != nil {
		fmt.Println(err.Error())
		return
	}
	fmt.Printf("send icmp packet success!")
}
