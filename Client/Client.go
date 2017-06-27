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

	ClienID int64

	chEnd chan bool
)

func init() {
	chEnd = make(chan bool)

	lockSend = new(sync.Mutex)

	log = util.Mylog

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

func doP2PConnect(bufRec []byte, lenBody int, proto string, addrLocal string) {
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

	log.Debugf("connect from: %s, to: %s : %d", addrLocal, strIP, ctc.Port)

	for i := 0; i < 10; i++ {
		err := ConnectClient(proto, addrLocal, strIP, int(ctc.Port))
		if err == nil {
			log.Error(err)
			break
		}
		time.Sleep(1 * time.Second)
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
	log.Debugf("Server conneted, RemoteAddr:%s, LocalAddr:%s", (*conn).RemoteAddr().String(), (*conn).LocalAddr().String())

	//接收
	go func() {
		bufRec := make([]byte, 256)
		_ = bufRec
		for {
			lenBody, typeBody, e := util.Receive(conn, bufRec)
			if e != nil {
				if e.Error() == "EOF" {
					(*conn).Close()
					log.Error("Socket Closed")
				} else {
					log.Error(e)
					errRet = e
				}
				return
			}
			if lenBody == 0 || typeBody == 0 {
				continue
			}

			//Client通知Server需要向某个ID发起连接：C-->S
			//Server 通知 被连接Client，有Client想要连接你，请尝试"铺路"：S-->C
			if typeBody == util.CONNECT {
				//				//关闭和服务器的连接
				//				chEnd <- true
				//				(*conn).Close()

				doP2PConnect(bufRec, lenBody, proto, (*conn).LocalAddr().String())

				return
				//continue
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
		for {
			select {
			case <-tickerSchedule.C:
				//log.Info("HeartBeat,data:", bufHeartBeat)
				util.SendWithLock(lockSend, conn, []byte("Keep-alive"), util.HEART_BEAT)
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
	log.Debugf("connected,RemoteAddr:%s,Local:%s", (*conn).RemoteAddr().String(), (*conn).LocalAddr().String())

	defer (*conn).Close()

	bufRec := make([]byte, 256)
	_ = bufRec

	//接收
	go func() {
		for {
			lenBody, typeBody, e := util.Receive(conn, bufRec)
			if e != nil {
				log.Error(e)
				return
			}
			if lenBody == 0 || typeBody == 0 {
				log.Error("received data len==0")
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
			//心跳包
			if typeBody == util.HEART_BEAT {
				log.Infof("HeartBeat,from:%s, data:%s", (*conn).RemoteAddr().String(), string(bufRec[:lenBody]))
				continue
			}
			log.Infof("received from:%s, data:%s", (*conn).RemoteAddr().String(), string(bufRec[:lenBody]))
		}
	}()

	//发送：NAT 一般 20s 断开映射
	tickerSchedule := time.NewTicker(8 * time.Second)
	bufHeartBeat := []byte("Keep-alive")
	util.SendWithLock(lockSend, conn, bufHeartBeat, util.HEART_BEAT)

	for {
		select {
		case <-tickerSchedule.C:
			log.Infof("HeartBeat,from:%s,to:%s,data:%s", (*conn).LocalAddr().String(), (*conn).LocalAddr().String(), string(bufHeartBeat))
			util.SendWithLock(lockSend, conn, bufHeartBeat, util.HEART_BEAT)

		case <-chEnd:
			tickerSchedule.Stop()
			log.Info("terminate!")
			log.Flush()
			return
		}

	}
}

func sendConnect(conn *net.Conn, idFrom, idTo int64) (errRet error) {
	bufSend := make([]byte, 0, 32)
	//|idFrom:8 Byte|--|idTo:8 Byte|--|PasswordLen: Byte|---|Password:PasswordLen Byte|

	x := uint(56)
	for i := 0; i < 8; i++ {
		b := byte((idFrom >> x) & 0xff)
		bufSend = append(bufSend, b)
		x -= 8
	}
	x = 56
	for i := 8; i < 16; i++ {
		b := byte((idTo >> x) & 0xff)
		bufSend = append(bufSend, b)
		x -= 8
	}
	bufSend = append(bufSend, byte(int8(len("password-test"))))
	bufSend = append(bufSend, []byte("password-test")...)

	log.Infof("SendToServer,idFrom:%d,idTo:%s", idFrom, idTo)
	//这里可以连接8088端口，，，aux
	return util.SendWithLock(lockSend, conn, bufSend, util.CONNECT)
}

func main() {
	//ConnectServer(util.CfgNet.Proto, util.CfgNet.Addr, util.CfgNet.ServerIP, 8088)

	log.Debug("ConnectServer:", util.CfgNet.Proto, ",", util.CfgNet.ServerIP, ":", util.CfgNet.ServerPort)
	//conn, err := ConnectServer(util.CfgNet.Proto, util.CfgNet.Addr, util.CfgNet.ServerIP, util.CfgNet.ServerPort)
	conn, err := ConnectServer(util.CfgNet.Proto, "", util.CfgNet.ServerIP, util.CfgNet.ServerPort)
	if err != nil {
		log.Error(err)
	}
	defer (*conn).Close()

	{
		//log.Debug("Listen:", util.CfgNet.Proto, ",", util.CfgNet.Addr)
		log.Debug("Listen:", util.CfgNet.Proto, ",", (*conn).LocalAddr().String())
		go Listen(util.CfgNet.Proto, (*conn).LocalAddr().String())

	}

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
			id, err := strconv.ParseInt(params[1], 10, 64)
			if err != nil {
				continue
			}
			//			if err := sendConnect(conn, ClienID, id); err != nil {
			//				return
			//			}

			if err := ConnectServerAndSendConnect(util.CfgNet.Proto, (*conn).LocalAddr().String(), util.CfgNet.ServerIP, 8088, ClienID, id); err != nil {
				log.Debug(err)
			}
		}
	}
}

func ConnectServerAndSendConnect(proto string, addr string, addrTo string, portTo int, idFrom, idTo int64) (errRet error) {
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
	log.Debugf("Server Aux conneted, RemoteAddr:%s, LocalAddr:%s", (*conn).RemoteAddr().String(), (*conn).LocalAddr().String())

	//
	bufSend := make([]byte, 0, 32)
	//|idFrom:8 Byte|--|idTo:8 Byte|--|PasswordLen: Byte|---|Password:PasswordLen Byte|

	x := uint(56)
	for i := 0; i < 8; i++ {
		b := byte((idFrom >> x) & 0xff)
		bufSend = append(bufSend, b)
		x -= 8
	}
	x = 56
	for i := 8; i < 16; i++ {
		b := byte((idTo >> x) & 0xff)
		bufSend = append(bufSend, b)
		x -= 8
	}
	bufSend = append(bufSend, byte(int8(len("password-test"))))
	bufSend = append(bufSend, []byte("password-test")...)

	log.Infof("SendToServer,idFrom:%d,idTo:%s", idFrom, idTo)
	//这里可以连接8088端口，，，aux
	errRet = util.Send(conn, bufSend, util.CONNECT)

	(*conn).Close()
	return
}
