package main

import (
	"net"
	"strings"
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
func TCPSocketServer(proto, addr string) {
	//建立socket，监听端口
	log.Debug(addr, " before Listen")
	netListen, err := net.Listen(proto, addr)
	if err != nil {
		log.Errorf("Fatal error: %s", err.Error())
		return
	}
	defer netListen.Close()

	var id int64
	for {
		conn, err := netListen.Accept()
		if err != nil {
			log.Error(err)
			continue
		}
		id++

		log.Debug(conn.RemoteAddr().String(), " tcp connect success")

		go afterAccept(&conn, id)
	}
}

func doConnect(conn *net.Conn, typeBody int8, id int64, bufRec []byte, lenBody int) {
	//|ID:8 Byte|--|Password:n Byte|
	cts := util.ConnectToServer{}
	for i := 0; i < 8; i++ {
		cts.ID = cts.ID << 8
		cts.ID = cts.ID | int64(bufRec[i])
	}
	cts.Password = bufRec[8:]

	connFrom, ok := dao.GetByID(id)
	if !ok {
		log.Error("connFrom ID is not Exist")
		return
	}
	connTo, ok := dao.GetByID(cts.ID)
	if !ok {
		log.Error("connTo ID is not Exist")
		return
	}

	//log.Infof("Src:%v, Connect To :%v", (*conn).RemoteAddr().String(), (*connTo.Conn).RemoteAddr().String())
	log.Infof("Src:%v, \n Connect To :%v", connFrom, connTo)

	addrFrom := strings.Split(connFrom.Addr, ":")
	if len(addrFrom) != 2 {
		log.Error("connFrom.Addr, err:", connFrom.Addr)
		return
	}

	ctc := util.ConnectToClient{
		IP:       util.InetAddr(addrFrom[0]),
		Port:     util.InetPort(addrFrom[1]),
		ID:       cts.ID,
		Password: cts.Password,
	}
	log.Debug(addrFrom, "---", ctc)

	bufSend := make([]byte, 0, 32)
	bufSend = append(bufSend, ctc.IP[:]...)

	bufSend = append(bufSend, byte((ctc.Port>>8)&0xff))
	bufSend = append(bufSend, byte(ctc.Port&0xff))

	bytesID := []byte{
		byte((ctc.ID >> 56) & 0xff),
		byte((ctc.ID >> 48) & 0xff),
		byte((ctc.ID >> 40) & 0xff),
		byte((ctc.ID >> 32) & 0xff),
		byte((ctc.ID >> 24) & 0xff),
		byte((ctc.ID >> 16) & 0xff),
		byte((ctc.ID >> 8) & 0xff),
		byte(ctc.ID & 0xff),
		//byte(typeB),
	}
	bufSend = append(bufSend, bytesID...)
	bufSend = append(bufSend, bufRec[:lenBody]...)
	util.SendWithLock(connFrom.Lock, connTo.Conn, bufSend, typeBody)

	//给源发送连接请求
	{
		addrTo := strings.Split(connTo.Addr, ":")
		if len(addrTo) != 2 {
			log.Error("connTo.Addr, err:", connTo.Addr)
			return
		}

		ctc := util.ConnectToClient{
			IP:       util.InetAddr(addrTo[0]),
			Port:     util.InetPort(addrTo[1]),
			ID:       cts.ID,
			Password: cts.Password,
		}
		log.Debug(addrTo, "---", ctc)

		bufSend := make([]byte, 0, 32)
		bufSend = append(bufSend, ctc.IP[:]...)

		bufSend = append(bufSend, byte((ctc.Port>>8)&0xff))
		bufSend = append(bufSend, byte(ctc.Port&0xff))

		bufSend = append(bufSend, bytesID...)
		bufSend = append(bufSend, bufRec[:lenBody]...)
		util.SendWithLock(connTo.Lock, connFrom.Conn, bufSend, typeBody)

	}
	return
}
func afterAccept(conn *net.Conn, id int64) {
	//把连接存起来，以后就可以使用了
	clientAddr := (*conn).RemoteAddr()
	connRet, err := dao.Add(clientAddr.String(), id, conn)
	if err != nil {
		log.Error(err)
		return
	}
	_ = connRet

	log.Debug("建立通信，开始处理！")
	//发送Client的ID
	{
		log.Infof("Send ID")
		bufSend := []byte{
			byte((id >> 56) & 0xff),
			byte((id >> 48) & 0xff),
			byte((id >> 40) & 0xff),
			byte((id >> 32) & 0xff),
			byte((id >> 24) & 0xff),
			byte((id >> 16) & 0xff),
			byte((id >> 8) & 0xff),
			byte(id & 0xff),
			//byte(typeB),
		}
		util.SendWithLock(connRet.Lock, connRet.Conn, bufSend, util.ID)
	}

	bufRec := make([]byte, 256)
	for {
		lenBody, typeBody, e := util.Receive(conn, bufRec)
		if e != nil {
			log.Error(e)
			//断开连接了，所以删除。。。
			if e.Error() == "EOF" {
				(*conn).Close()
				dao.Remove(id)
			} else {
				log.Error(",,,,")
			}
			return
		}
		if lenBody == 0 || typeBody == 0 {
			continue
		}

		//心跳包
		if typeBody == util.HEART_BEAT {
			log.Infof("HeartBeat,data:%s", string(bufRec[:lenBody]))
			continue
		}

		//Client通知Server需要向某个ID发起连接：C-->S
		//Server 通知 被连接Client，有Client想要连接你，请尝试"铺路"：S-->C
		if typeBody == util.CONNECT {
			doConnect(conn, typeBody, id, bufRec, lenBody)
		}

		//通知Server，Client需要获取自己的ID：C-->S
		//通知Client，这是你的ID：S-->C
		if typeBody == util.ID {
			log.Infof("ID,data:%s", string(bufRec[:lenBody]))

			bufSend := []byte{
				byte((id >> 56) & 0xff),
				byte((id >> 48) & 0xff),
				byte((id >> 40) & 0xff),
				byte((id >> 32) & 0xff),
				byte((id >> 24) & 0xff),
				byte((id >> 16) & 0xff),
				byte((id >> 8) & 0xff),
				byte(id & 0xff),
				//byte(typeB),
			}

			util.Send(conn, bufSend, util.ID)
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

func main() {
	log.Debug("start:", util.CfgNet.Proto, "  ", util.CfgNet.Addr)
	//TCP监听
	TCPSocketServer(util.CfgNet.Proto, util.CfgNet.Addr)

	log.Flush()
}
