package main

import (
	"net"
	"sync"
	"time"

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
func TCPSocket(port string) {
	//建立socket，监听端口
	netListen, err := net.Listen("tcp", port)
	if err != nil {
		log.Errorf("Fatal error: %s", err.Error())
		return
	}
	defer netListen.Close()

	for {
		conn, err := netListen.Accept()
		if err != nil {
			continue
		}

		log.Debug(conn.RemoteAddr().String(), " tcp connect success")
		if connClient != nil {
			log.Errorf("断开客户端链接IP:%s，链接新客户端:%s", (*connClient).RemoteAddr(), conn.RemoteAddr())
			(*connClient).Close()
		}
		connClient = &conn
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

func Listen(proto string, addr string) {
	socket, err := util.Socket(proto, addr)
	if err != nil {
		log.Error(err)
		return
	}
	util.Listen(socket, func(conn *net.Conn) (err error) {
		log.Debug("get date")

		buffer := make([]byte, 204800)
		//自定义Socket头： ----[4byte长度] ----[1byte类型，3byte纠错标识符："LXT"]
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
			if typeBody != 1 && typeBody != 0 && typeBody != 8 {
				log.Debug("typeBody error")
				continue
			}

			//心跳包
			if typeBody == 8 {
				log.Infof("HeartBeat,data:%s", string(buffer[:lenBody]))
			}
		}
		return nil
	})
}

func Connect(proto string, addr string, addrTo string, portTo int) {
	socket, err := util.Socket(proto, addr)
	if err != nil {
		log.Error(err)
		return
	}
	conn, err := util.Connect(socket, addrTo, portTo)
	if err != nil {
		log.Error(err)
		return
	}

	defer (*conn).Close()

	//conn.Write("bufRequestHeader.Bytes()")
	//NAT 一般 20s 断开映射
	tickerSchedule := time.NewTicker(18 * time.Second)
	body := "Keep-alive"
	for {
		send2Client(conn, body, 8)

		//定时器，每个固定时间触发一次
		<-tickerSchedule.C
		log.Info("HeartBeat,data:", body)

	}
}

func main() {
	log.Debug("start")
	//TCP监听
	//go TCPSocket(":8082")
	go Listen("tcp", ":8081")

	time.Sleep(10 * time.Second)

	Connect("tcp", ":8082", "192.168.3.91", 8081)

	return

	//心跳包
	go func() {
		//NAT 一般 20s 断开映射
		tickerSchedule := time.NewTicker(18 * time.Second)
		body_str := "Keep-alive"
		for {
			send2Client(connClient, body_str, 8)

			//定时器，每个固定时间触发一次
			<-tickerSchedule.C
			log.Info("HeartBeat,data:", body_str)
		}
	}()

}
