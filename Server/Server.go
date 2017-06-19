package main

import (
	"net"
	"sync"
	//	"sync/atomic"
	"time"

	"github.com/lxt1045/TCPHolePunching/Server/dao"
	"github.com/lxt1045/TCPHolePunching/util"
)

var log util.MylogStruct
var lockSend *sync.Mutex
var connClient *net.Conn
var local *time.Location

func init() {
	lockSend = new(sync.Mutex)

	log = util.Mylog

	local, _ := time.LoadLocation("Asia/Chongqing")
	_ = local
}

//建立TCP连接
func TCPSocketServer(addr string) {
	//建立socket，监听端口
	netListen, err := net.Listen("tcp", addr)
	if err != nil {
		log.Errorf("Fatal error: %s", err.Error())
		return
	}
	defer netListen.Close()

	var id int64
	for {
		conn, err := netListen.Accept()
		if err != nil {
			continue
		}
		id++

		log.Debug(conn.RemoteAddr().String(), " tcp connect success")

		go afterAccept(&conn, id)
	}
}

func Read(conn *net.Conn, buffer []byte) (l, t int, errRet error) {
	//读取长度
	n, err := (*conn).Read(header)
	if err != nil {
		log.Debugf("connection error:%s", err.Error())
		errRet = err
		return
	}
	if n != 8 {
		//读取出错
		log.Debug("socket read error")
		return
	}
	if string(header[5:]) != "LXT" {
		//校验出错
		log.Debug("Checksum error")
		return
	}
	//读取主体
	n1, err := (*conn).Read(buffer)
	if err != nil {
		log.Debugf("connection error:%s", err.Error())
		errRet = err
		return
	}
	log.Debugf("IP:%s, receive data string:%s\n", (*conn).RemoteAddr().String(), string(buffer[:n1]))

	lenBody := int(header[0])*0x1000000 + int(header[1])*0x10000 + int(header[2])*0x100 + int(header[3])
	if lenBody <= 0 || lenBody > len(buffer) || lenBody != n1 {
		log.Debug("socket data error")
		return
	}

	typeBody := int(header[4])

	return lenBody, typeBody, nil
}
func afterAccept(conn *net.Conn, id int64) {
	clientAddr := (*conn).RemoteAddr()
	connRet, err := dao.Add(clientAddr.String(), id, conn)
	if err != nil {
		log.Error(err)
		return
	}

	log.Debug("建立通信，开始处理！")

	buffer := make([]byte, 256)
	//自定义Socket头，一共8Byte： ----[4byte长度] ----[1byte类型，3byte纠错标识符："LXT"]
	header := make([]byte, 8)

	for {
		//读取长度
		n, err := (*conn).Read(header)
		if err != nil {
			log.Debugf("connection error:%s", err.Error())
			break
		}
		if n != 8 {
			//读取出错
			log.Debug("socket read error")
			continue
		}
		if string(header[5:]) != "LXT" {
			//校验出错
			log.Debug("Checksum error")
			continue
		}
		//读取主体
		n1, err := (*conn).Read(buffer)
		if err != nil {
			log.Debugf("connection error:%s", err.Error())
			break
		}
		log.Debugf("IP:%s, receive data string:%s\n", (*conn).RemoteAddr().String(), string(buffer[:n1]))

		lenBody := int(header[0])*0x1000000 + int(header[1])*0x10000 + int(header[2])*0x100 + int(header[3])
		if lenBody <= 0 || lenBody > len(buffer) || lenBody != n1 {
			log.Debug("socket data error")
			continue
		}

		//以下根据类型调用rpc
		typeBody := int(header[4])

		//心跳包
		if typeBody == util.HEART_BEAT {
			log.Infof("HeartBeat,data:%s", string(buffer[:lenBody]))
			continue
		}

		//Client通知Server需要向某个ID发起连接：C-->S
		//Server 通知 被连接Client，有Client想要连接你，请尝试"铺路"：S-->C
		if typeBody == util.CONNECT {
			log.Infof("CONNECT,data:%s", string(buffer[:lenBody]))
			continue
		}

		//通知Server，Client需要获取自己的ID：C-->S
		//通知Client，这是你的ID：S-->C
		if typeBody == util.ID {
			log.Infof("ID,data:%s", string(buffer[:lenBody]))
			continue
		}

		//其他情况
		if typeBody != 1 && typeBody != 0 && typeBody != 8 {
			log.Debug("typeBody error")
			continue
		}
	}
}

//处理连接
func send2Client(conn *net.Conn, body string, typeB int32) {
	if conn == nil {
		return
	}

	lenBody := int32(len(body))
	//自定义Socket头： ----[4byte长度] ----[1byte类型，3byte纠错标识符："LXT"]
	header := []byte{
		byte((lenBody >> 24) & 0xff),
		byte((lenBody >> 16) & 0xff),
		byte((lenBody >> 8) & 0xff),
		byte(lenBody & 0xff),
		byte(typeB),
		'L',
		'X',
		'T',
	}
	//lockSend.Lock()

	(*conn).Write(header)
	(*conn).Write([]byte(body))

	//lockSend.Unlock()
}

func main() {
	log.Debug("start")
	//TCP监听
	TCPSocketServer(":8082")

}
