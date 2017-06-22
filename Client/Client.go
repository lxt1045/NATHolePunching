package main

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/lxt1045/TCPHolePunching/util"
)

var (
	log      util.MylogStruct
	lockSend *sync.Mutex
	local    *time.Location

	ClienID int64

	connClient *net.Conn

	chEnd chan bool
)

func init() {
	chEnd = make(chan bool)

	lockSend = new(sync.Mutex)

	log = util.Mylog

	local, _ := time.LoadLocation("Asia/Chongqing")
	_ = local
}

func Listen(proto string, addr string) {
	socket, err := util.Socket(proto, addr)
	if err != nil {
		log.Error(err)
		return
	}
	util.Listen(socket, func(conn *net.Conn) (err error) {

		buffer := make([]byte, 204800)

		for {
			lenBody, typeBody, e := util.Receive(conn, buffer)
			if e != nil {
				log.Error(e)
				return
			}
			log.Debugf("get date:%s,typeBody:%d", string(buffer[:lenBody]), typeBody)

			if lenBody == 0 || typeBody == 0 {
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

//try==true：被动连前的“探路”
func ConnectServer(proto string, addr string, addrTo string, portTo int, scout bool) {
	log.Debugf("proto:%s, addr:%s, addrTo:%s, portTo:%d, scout:%t", proto, addr, addrTo, portTo, scout)

	socket, err := util.Socket(proto, addr)
	if err != nil {
		log.Error(err)
		return
	}
	conn, err := util.Connect(socket, util.InetAddr(addrTo), portTo)
	if err != nil {
		log.Error(err)
		return
	}
	log.Debug("after conneted:", (*conn).RemoteAddr().String())

	defer (*conn).Close()
	if scout {
		return
	}

	connClient = conn
	bufRec := make([]byte, 256)
	_ = bufRec

	//接收
	go func() {
		for {
			log.Debug("before Receive")
			lenBody, typeBody, e := util.Receive(conn, bufRec)
			if e != nil {
				log.Error(e)
				return
			}
			if lenBody == 0 || typeBody == 0 {
				continue
			}

			//Client通知Server需要向某个ID发起连接：C-->S
			//Server 通知 被连接Client，有Client想要连接你，请尝试"铺路"：S-->C
			if typeBody == util.CONNECT {
				log.Infof("CONNECT,data:%s", string(bufRec[:lenBody]))

				ctc := util.ConnectToClient{
					//IP:   [4]byte(bufRec[:4]),
					Port: (uint16(bufRec[4]) << 8) | (uint16(bufRec[5])),
					//ID:       cts.ID,
					Password: bufRec[4+2+8:],
				}
				strIP := fmt.Sprintf("%d.%d.%d.%d", bufRec[0], bufRec[1], bufRec[2], bufRec[3])

				log.Debug("connect to: %s : %d", strIP, ctc.Port)

				for i := 0; i < len(ctc.IP); i++ {
					ctc.IP[i] = bufRec[i]
				}
				for i := 0; i < 8; i++ {
					ctc.ID = ctc.ID << 8
					ctc.ID = ctc.ID | int64(bufRec[4+2+i])
				}
				if ClienID == 0 || ctc.ID != ClienID {
					//自己主动发起的连接
					time.Sleep(1 * time.Second)
					ConnectClient(proto, addr, strIP, int(ctc.Port), false)
				} else if ctc.ID == ClienID {
					//给对方“铺路”
					time.Sleep(1 * time.Second)
					ConnectClient(proto, addr, strIP, int(ctc.Port), true)
				}

				continue
			}

			//通知Server，Client需要获取自己的ID：C-->S
			//通知Client，这是你的ID：S-->C
			if typeBody == util.ID {
				//log.Infof("ID,data:%s", string(bufRec[:lenBody]))
				if lenBody != 8 {
					continue
				}
				var ID int64
				for i := 0; i < lenBody; i++ {
					ID = ID << 8
					ID = ID | int64(bufRec[i])
				}

				ClienID = ID

				log.Info("ID:", ID)
				continue
			}
		}
	}()

	//发送：NAT 一般 20s 断开映射
	tickerSchedule := time.NewTicker(18 * time.Second)
	tickerSchedule1 := time.NewTicker(3 * time.Second)
	bufHeartBeat := []byte("Keep-alive")
	for {
		log.Debug("before HeartBeat")
		select {
		case <-tickerSchedule.C:
			log.Info("HeartBeat,data:", bufHeartBeat)
			util.SendWithLock(lockSend, conn, bufHeartBeat, util.HEART_BEAT)
		case <-tickerSchedule1.C:
			tickerSchedule1.Stop()

			//|ID:8 Byte|--|Password:n Byte|
			cts := util.ConnectToServer{
				ID:       1,
				Password: []byte("password"),
			}
			bytesID := []byte{
				byte((cts.ID >> 56) & 0xff),
				byte((cts.ID >> 48) & 0xff),
				byte((cts.ID >> 40) & 0xff),
				byte((cts.ID >> 32) & 0xff),
				byte((cts.ID >> 24) & 0xff),
				byte((cts.ID >> 16) & 0xff),
				byte((cts.ID >> 8) & 0xff),
				byte(cts.ID & 0xff),
				//byte(typeB),
			}
			bufSend := make([]byte, 0, 32)
			bufSend = append(bufSend, bytesID...)
			bufSend = append(bufSend, cts.Password...)

			//
			log.Infof("ConnectToServer,ID:%d,Password:%s", cts.ID, string(cts.Password))
			util.SendWithLock(lockSend, conn, bufSend, util.CONNECT)

		case <-chEnd:
			tickerSchedule.Stop()
			log.Info("terminate!")
			log.Flush()
			return
		}

	}
}

//try==true：被动连前的“探路”
func ConnectClient(proto string, addr string, addrTo string, portTo int, scout bool) {
	log.Debugf("proto:%s, addr:%s, addrTo:%s, portTo:%d, scout:%t", proto, addr, addrTo, portTo, scout)

	socket, err := util.Socket(proto, addr)
	if err != nil {
		log.Error("Socket err:", err)
		return
	}

	conn, err := util.Connect(socket, util.InetAddr(addrTo), portTo)
	if err != nil {
		log.Error("ConnectClient: ", err)
		return
	}
	log.Debug("after connected,RemoteAddr:%s,Local:%s", (*conn).RemoteAddr().String(), (*conn).LocalAddr().String())

	defer (*conn).Close()
	if scout {
		return
	}

	bufRec := make([]byte, 256)
	_ = bufRec

	//接收
	go func() {
		for {
			log.Debug("before Receive")
			lenBody, typeBody, e := util.Receive(conn, bufRec)
			if e != nil {
				log.Error(e)
				return
			}
			if lenBody == 0 || typeBody == 0 {
				continue
			}
			//Client 间相互发消息
			if typeBody == util.DATA {
				log.Infof("DATA,data:%s", string(bufRec[:lenBody]))
				if lenBody != 8 {
					continue
				}
				var ID int64
				for i := 0; i < lenBody; i++ {
					ID = ID << 8
					ID = ID | int64(bufRec[i])
				}

				log.Info("ID:", ID)
				continue
			}
			log.Infof("data:%s", string(bufRec[:lenBody]))
		}
	}()

	//发送：NAT 一般 20s 断开映射
	tickerSchedule := time.NewTicker(3 * time.Second)
	bufHeartBeat := []byte("Keep-alive")
	for {
		log.Debug("before HeartBeat")
		select {
		case <-tickerSchedule.C:
			log.Info("HeartBeat,data:", bufHeartBeat)
			util.SendWithLock(lockSend, conn, bufHeartBeat, util.HEART_BEAT)

		case <-chEnd:
			tickerSchedule.Stop()
			log.Info("terminate!")
			log.Flush()
			return
		}

	}
}

func main() {
	//log.Debug("start")
	//TCP监听

	log.Debug("Listen:", util.CfgNet.Proto, ",", util.CfgNet.Addr)
	go Listen(util.CfgNet.Proto, util.CfgNet.Addr)

	//time.Sleep(3 * time.Second)

	log.Debug("ConnectServer:", util.CfgNet.Proto, ",", util.CfgNet.Addr)
	//ConnectServer(util.CfgNet.Proto, util.CfgNet.Addr, "192.168.3.91", 8082, false)
	ConnectServer(util.CfgNet.Proto, util.CfgNet.Addr, util.CfgNet.ServerIP, util.CfgNet.ServerPort, false)

}
