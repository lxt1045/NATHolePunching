package main

import (
	"net"
	"strings"
	"sync"
	//	"sync/atomic"
	"bytes"
	"encoding/binary"
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
func TCPSocketServer(proto, addr string, aux bool) {
	//建立socket，监听端口
	log.Debug(addr, "Listening")
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
		if aux {
			go afterAcceptAux(&conn)
		} else {
			go afterAccept(&conn, id, aux)
		}
	}
}

func afterAccept(conn *net.Conn, id int64, aux bool) {
	//把连接存起来，以后就可以使用了
	clientAddr := (*conn).RemoteAddr()
	connRet, err := dao.Add(clientAddr.String(), id, conn)
	if err != nil {
		log.Error(err)
		return
	}
	_ = connRet

	//log.Debug("建立通信，开始处理！")
	//发送Client的ID
	{
		log.Infof("Send ID:%d", id)
		var buffer bytes.Buffer
		if err := binary.Write(&buffer, binary.BigEndian, id); err != nil {
			log.Error(err)
			return
		}
		util.SendWithLock(connRet.Lock, connRet.Conn, buffer.Bytes(), util.ID)
	}

	bufRec := make([]byte, 256)
	for {
		lenBody, typeBody, e := util.Receive(conn, bufRec)
		if e != nil {
			//断开连接了，所以删除。。。
			if e.Error() == "EOF" {
				(*conn).Close()
				log.Error("Socket Closed")
				dao.Remove(id)
			} else {
				log.Error("非EOF网络错误:", e)
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
		if typeBody == util.CONNECT_DST || typeBody == util.CONNECT_SRC {
			//按步骤走，先给dst端发送数据包，处理预连接
			var connInfo util.CreateConn
			buffer := bytes.NewBuffer(bufRec)
			if err := binary.Read(buffer, binary.BigEndian, &connInfo); err != nil {
				log.Error(err)
				return
			}
			doConnect(conn, typeBody, &connInfo)
		}

		//通知Server，Client需要获取自己的ID：C-->S
		//通知Client，这是你的ID：S-->C
		if typeBody == util.ID {
			log.Infof("ID,data:%s", string(bufRec[:lenBody]))

			var buffer bytes.Buffer
			if err := binary.Write(&buffer, binary.BigEndian, id); err != nil {
				log.Error(err)
				continue
			}
			util.SendWithLock(connRet.Lock, connRet.Conn, buffer.Bytes(), util.ID)
			continue
		}
	}
}

func afterAcceptAux(conn *net.Conn) {
	//把连接存起来，以后就可以使用了
	//clientAddr := (*conn).RemoteAddr()

	log.Debugf("建立通信,SRC:%s,TG:%s", (*conn).LocalAddr().String(), (*conn).RemoteAddr().String())

	bufRec := make([]byte, 256)
	for {
		lenBody, typeBody, e := util.Receive(conn, bufRec)
		if e != nil {
			//断开连接了，所以删除。。。
			if e.Error() == "EOF" {
				(*conn).Close()
				log.Errorf("关闭连接,SRC:%s,TG:%s", (*conn).LocalAddr().String(), (*conn).RemoteAddr().String())
			} else {
				log.Error("非EOF网络错误:", e)
			}
			return
		}
		if lenBody == 0 || typeBody == 0 {
			continue
		}

		//Client通知Server需要向某个ID发起连接：C-->S
		//Server 通知 被连接Client，有Client想要连接你，请尝试"铺路"：S-->C
		if typeBody == util.CONNECT_AUX {
			var id int64
			buffer := bytes.NewBuffer(bufRec)
			if err := binary.Read(buffer, binary.BigEndian, &id); err != nil {
				log.Error(err)
				return
			}

			doConnectAux(conn, id)
		}

	}
}

func doConnect(conn *net.Conn, typeBody int8, connInfo *util.CreateConn) {
	connFrom, ok1 := dao.GetByID(connInfo.IDFrom)
	connTo, ok2 := dao.GetByID(connInfo.IDTo)
	if !ok1 || !ok2 {
		log.Errorf("ID is not Exist,ok1:%t,ok2:%t", ok1, ok2)
		return
	}

	log.Infof("Src:%v, \n Connect To :%v", connFrom.Addr, connTo.Addr)

	if typeBody == util.CONNECT_SRC {
		addrFrom := strings.Split(connFrom.Addr, ":")
		if len(addrFrom) != 2 {
			log.Error("connFrom.Addr, err:", connFrom.Addr)
			return
		}

		//主动发情请求的的一方的信息
		infoFrom := util.ConnectToClient{
			IP:       util.InetAddr(addrFrom[0]),
			Port:     util.InetPort(addrFrom[1]),
			ID:       connInfo.IDFrom,
			Password: connInfo.Password,
		}
		var buffer bytes.Buffer
		if err := binary.Write(&buffer, binary.BigEndian, infoFrom); err != nil {
			log.Error(err)
			return
		}
		util.SendWithLock(connTo.Lock, connTo.Conn, buffer.Bytes(), util.CONNECT_DST)

	} else if typeBody == util.CONNECT_DST {
		addrTo := strings.Split(connTo.Addr, ":")
		if len(addrTo) != 2 {
			log.Error("connTo.Addr, err:", connTo.Addr)
			return
		}

		infoTo := util.ConnectToClient{
			IP:       util.InetAddr(addrTo[0]),
			Port:     util.InetPort(addrTo[1]),
			ID:       connInfo.IDTo,
			Password: connInfo.Password,
		}

		var buffer bytes.Buffer
		if err := binary.Write(&buffer, binary.BigEndian, infoTo); err != nil {
			log.Error(err)
			return
		}
		util.SendWithLock(connFrom.Lock, connFrom.Conn, buffer.Bytes(), util.CONNECT_SRC)

	}
	return
}

func doConnectAux(conn *net.Conn, id int64) {
	connRemote, ok := dao.GetByID(id)
	if !ok {
		log.Errorf("Addr:%s is not Exist!", (*conn).RemoteAddr().String())
		return
	}
	connRemote.Lock.Lock()
	if connRemote.Addr == (*conn).RemoteAddr().String() {
		connRemote.ClientType = 2
	} else {
		log.Errorf("ClientAddr:%s is not same as Addr:%s!", (*conn).RemoteAddr().String(), connRemote.Addr)
		connRemote.ClientType = 3
	}
	connRemote.Lock.Unlock()
	return
}

//处理连接

func main() {
	log.Debug("start:", util.CfgNet.Proto, "  ", util.CfgNet.Addr)

	go TCPSocketServer(util.CfgNet.Proto, ":8088", true)
	//TCP监听
	TCPSocketServer(util.CfgNet.Proto, util.CfgNet.Addr, false)

	log.Flush()
}
