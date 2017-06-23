package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
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
	err = util.Listen(socket, func(conn *net.Conn) (err error) {

		buffer := make([]byte, 2048)

		for {
			lenBody, typeBody, e := util.Receive(conn, buffer)
			if e != nil {
				log.Error(e)
				return
			}
			log.Debugf("Receive:%s,type:%d", string(buffer[:lenBody]), typeBody)

			if lenBody == 0 || typeBody == 0 {
				continue
			}

			//心跳包
			if typeBody == util.HEART_BEAT {
				log.Infof("HeartBeat,data:%s", string(buffer[:lenBody]))
			}
		}
		return nil
	})
	if err != nil {
		log.Error(err)
	}
}

func doP2PConnect(bufRec []byte, lenBody int, proto string, addr string) {
	log.Infof("CONNECT,data:%s", string(bufRec[:lenBody]))

	ctc := util.ConnectToClient{
		//IP:   [4]byte(bufRec[:4]),
		//ID:       cts.ID,
		Port:     (uint16(bufRec[4]) << 8) | (uint16(bufRec[5])),
		Password: bufRec[4+2+8:],
	}
	strIP := fmt.Sprintf("%d.%d.%d.%d", bufRec[0], bufRec[1], bufRec[2], bufRec[3])
	for i := 0; i < len(ctc.IP); i++ {
		ctc.IP[i] = bufRec[i]
	}
	for i := 0; i < 8; i++ {
		ctc.ID = ctc.ID << 8
		ctc.ID = ctc.ID | int64(bufRec[4+2+i])
	}

	log.Debugf("connect to: %s : %d", strIP, ctc.Port)

	for i := 0; i < 10; i++ {
		if ClienID == 0 || ctc.ID != ClienID {
			//自己主动发起的连接
			time.Sleep(1 * time.Second)
			err := ConnectClient(proto, addr, strIP, int(ctc.Port))
			if err == nil {
				break
			}
		} else if ctc.ID == ClienID {
			//给对方“铺路”
			time.Sleep(1 * time.Second)
			err := ConnectClient(proto, addr, strIP, int(ctc.Port))
			if err == nil {
				break
			}
		}
	}
}

func ConnectServer(proto string, addr string, addrTo string, portTo int) (connRet *net.Conn, errRet error) {
	log.Debugf("proto:%s, addr:%s, addrTo:%s, portTo:%d", proto, addr, addrTo, portTo)

	socket, err := util.Socket(proto, addr)
	if err != nil {
		log.Error(err)
		errRet = err
		return
	}
	conn, err := util.Connect(socket, util.InetAddr(addrTo), portTo)
	if err != nil {
		log.Error(err)
		errRet = err
		return
	}
	log.Debug("Server conneted, RemoteAddr:%s, LocalAddr:%s", (*conn).RemoteAddr().String(), (*conn).LocalAddr().String())

	connClient = conn

	//接收
	go func() {
		bufRec := make([]byte, 256)
		_ = bufRec
		for {
			lenBody, typeBody, e := util.Receive(conn, bufRec)
			if e != nil {
				log.Error(e)
				errRet = e
				return
			}
			if lenBody == 0 || typeBody == 0 {
				continue
			}

			//Client通知Server需要向某个ID发起连接：C-->S
			//Server 通知 被连接Client，有Client想要连接你，请尝试"铺路"：S-->C
			if typeBody == util.CONNECT {
				doP2PConnect(bufRec, lenBody, proto, addr)
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
	go func() {
		tickerSchedule := time.NewTicker(18 * time.Second)
		tickerSchedule1 := time.NewTicker(3 * time.Second)
		bufHeartBeat := []byte("Keep-alive")
		for {
			select {
			case <-tickerSchedule.C:
				//log.Info("HeartBeat,data:", bufHeartBeat)
				util.SendWithLock(lockSend, conn, bufHeartBeat, util.HEART_BEAT)
			case <-tickerSchedule1.C:
				tickerSchedule1.Stop()

			case <-chEnd:
				tickerSchedule.Stop()
				log.Info("terminate!")
				log.Flush()
				return
			}

		}
	}()

	return conn, nil
}

func ConnectClient(proto string, addr string, addrTo string, portTo int) (errRet error) {
	log.Debugf("proto:%s, addr:%s, addrTo:%s, portTo:%d", proto, addr, addrTo, portTo)

	socket, err := util.Socket(proto, addr)
	if err != nil {
		log.Error("Socket err:", err)
		return
	}
	conn, err := util.Connect(socket, util.InetAddr(addrTo), portTo)
	if err != nil {
		log.Error("ConnectClient: ", err)
		util.CloseSocket(socket)
		errRet = err
		return
	}
	log.Debugf("after connected,RemoteAddr:%s,Local:%s", (*conn).RemoteAddr().String(), (*conn).LocalAddr().String())

	defer (*conn).Close()

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

func sendConnect(conn *net.Conn, id int64) {
	//|ID:8 Byte|--|Password:n Byte|
	cts := util.ConnectToServer{
		ID:       id,
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

	log.Infof("SendToServer,ID:%d,Password:%s", cts.ID, string(cts.Password))
	util.SendWithLock(lockSend, conn, bufSend, util.CONNECT)
}
func main() {
	log.Debug("Listen:", util.CfgNet.Proto, ",", util.CfgNet.Addr)
	go Listen(util.CfgNet.Proto, util.CfgNet.Addr)

	//ConnectServer(util.CfgNet.Proto, util.CfgNet.Addr, util.CfgNet.ServerIP, 8088)

	log.Debug("ConnectServer:", util.CfgNet.Proto, ",", util.CfgNet.ServerIP, ":", util.CfgNet.ServerPort)
	conn, err := ConnectServer(util.CfgNet.Proto, util.CfgNet.Addr, util.CfgNet.ServerIP, util.CfgNet.ServerPort)
	if err != nil {
		log.Error(err)
	}
	defer (*conn).Close()

	reader := bufio.NewReader(os.Stdin)
	for {
		strBytes, _, err := reader.ReadLine()
		if err != nil {
			log.Error(err)
		}
		params := strings.Split(string(strBytes), " ")
		if len(params) < 2 {
			continue
		}
		switch params[0] {
		case "connect":
			id, err := strconv.ParseInt(params[1], 16, 64)
			if err != nil {
				continue
			}
			sendConnect(conn, id)
		}

	}
}
