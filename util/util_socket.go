package util

import (
	"net"
	"sync"
)

const (
	HEART_BEAT = 1 //心跳包：C-->S

	//Client通知Server需要向某个ID发起连接：C-->S
	//Server 通知 被连接Client，有Client想要连接你，请尝试"铺路"：S-->C
	CONNECT = 2

	//通知Server，Client需要获取自己的ID：C-->S
	//通知Client，这是你的ID：S-->C
	ID = 3

	//Client之间传输的数据
	DATA = 4
)

func init() {

}

type ConnectToServer struct {
	ID       int64
	Password []byte
}

type ConnectToClient struct {
	IP       [4]byte
	Port     uint16
	ID       int64
	Password []byte
}

func Send(conn *net.Conn, buffer []byte, t int8) (errRet error) {
	if conn == nil {
		return
	}

	lenBody := int32(len(buffer))
	//自定义Socket头： ----[4byte长度] ----[1byte类型，3byte纠错标识符："LXT"]
	header := []byte{
		byte((lenBody >> 24) & 0xff),
		byte((lenBody >> 16) & 0xff),
		byte((lenBody >> 8) & 0xff),
		byte(lenBody & 0xff),
		byte(t),
		'L',
		'X',
		'T',
	}

	if n, err := (*conn).Write(header); n != 8 || err != nil {
		errRet = err
	}
	if n, err := (*conn).Write(buffer); n != int(lenBody) || err != nil {
		errRet = err
	}
	return
}

func SendWithLock(lock *sync.Mutex, conn *net.Conn, buffer []byte, t int8) (errRet error) {
	if conn == nil {
		return
	}

	lenBody := int32(len(buffer))
	//自定义Socket头： ----[4byte长度] ----[1byte类型，3byte纠错标识符："LXT"]
	header := []byte{
		byte((lenBody >> 24) & 0xff),
		byte((lenBody >> 16) & 0xff),
		byte((lenBody >> 8) & 0xff),
		byte(lenBody & 0xff),
		byte(t),
		'L',
		'X',
		'T',
	}
	lock.Lock()

	if n, err := (*conn).Write(header); n != 8 || err != nil {
		errRet = err
	}
	if n, err := (*conn).Write(buffer); n != int(lenBody) || err != nil {
		errRet = err
	}
	lock.Unlock()
	return
}

func Receive(conn *net.Conn, buffer []byte) (l int, t int8, errRet error) {
	//自定义Socket头，一共8Byte： ----[4byte长度] ----[1byte类型，3byte纠错标识符："LXT"]
	header := make([]byte, 8)

	//读取长度
	n, err := (*conn).Read(header)
	if err != nil {
		Mylog.Debugf("connection error:%s", err.Error())
		errRet = err
		return
	}
	if n != 8 {
		//读取出错
		Mylog.Debug("socket read error")
		return
	}
	if string(header[5:]) != "LXT" {
		//校验出错
		Mylog.Debug("Checksum error")
		return
	}
	//读取主体
	n1, err := (*conn).Read(buffer)
	if err != nil {
		Mylog.Debugf("connection error:%s", err.Error())
		errRet = err
		return
	}
	Mylog.Debugf("IP:%s, receive data string:%s", (*conn).RemoteAddr().String(), string(buffer[:n1]))

	lenBody := int(header[0])*0x1000000 + int(header[1])*0x10000 + int(header[2])*0x100 + int(header[3])
	if lenBody <= 0 || lenBody > len(buffer) || lenBody != n1 {
		Mylog.Debug("socket data error")
		return
	}

	typeBody := int8(header[4])

	return lenBody, typeBody, nil
}
