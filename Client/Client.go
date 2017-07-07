package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
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

	x bool
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

func doP2PConnect(connInfo *util.ConnectToClient, lenBody int, proto string, addrLocal string, connType int8) {
	log.Infof("CONNECT,data:%v", connInfo)

	bufIP := connInfo.IP
	strIP := fmt.Sprintf("%d.%d.%d.%d", bufIP[0], bufIP[1], bufIP[2], bufIP[3])

	log.Debugf("connect from: %s, to: %s : %d", addrLocal, strIP, connInfo.Port)

	//
	if connType == util.CONNECT_SRC {
		err := ConnectClient(proto, addrLocal, strIP, int(connInfo.Port), false)
		if err == nil {
			log.Error(err)
		}
	} else if connType == util.CONNECT_DST {
		err := ConnectClient(proto, addrLocal, strIP, int(connInfo.Port), true)
		if err == nil {
			log.Error(err)
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
			if typeBody == util.CONNECT_SRC || typeBody == util.CONNECT_DST {

				connInfo := util.ConnectToClient{}
				buffer := bytes.NewBuffer(bufRec)
				if err := binary.Read(buffer, binary.BigEndian, &connInfo); err != nil {
					log.Error(err)
					continue
				}

				doP2PConnect(&connInfo, lenBody, proto, (*conn).LocalAddr().String(), typeBody)

				if typeBody == util.CONNECT_DST {
					if err := sendConnect(conn, connInfo.ID, ClienID, util.CONNECT_DST); err != nil {
						log.Debug(err)
					}
					continue
				}
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
func DialTimeout(network, localAddr, address string, timeout time.Duration) (net.Conn, error) {
	ra, err := net.ResolveTCPAddr("tcp4", localAddr)
	if err != nil {
		log.Error(err)
	}
	d := net.Dialer{
		Timeout:   timeout,
		LocalAddr: ra, //net.TCPAddr{},
	}
	return d.Dial(network, address)
}
func ConnectClient(proto string, addr string, addrTo string, portTo int, toread bool) (errRet error) {
	log.Debugf("proto:%s, addr:%s, addrTo:%s, portTo:%d", proto, addr, addrTo, portTo)

	if false {
		conn, err := DialTimeout("tcp", addr, fmt.Sprintf("%s:%d", addrTo, portTo), 3*time.Millisecond)
		if err != nil {
			log.Error(err.Error())
			//return err
		}
		if conn != nil {
			go conn.Write([]byte("1234567890"))
			buf := make([]byte, 12)
			go conn.Read(buf)
			log.Debugf("src:%s,dst:%s,data:%s", conn.LocalAddr().String(), conn.RemoteAddr().String(), string(buf))
			time.Sleep(5 * time.Second)
			conn.Close()
		}
	}

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
	if toread {
		return
	}

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

func sendConnect(conn *net.Conn, idFrom, idTo int64, connType int8) (errRet error) {
	//|idFrom:8 Byte|--|idTo:8 Byte|--|PasswordLen: Byte|---|Password:PasswordLen Byte|
	connInfo := util.CreateConn{
		IDFrom: idFrom,
		IDTo:   idTo,
		//Password: [12]byte{"password-test"},
	}
	copy(connInfo.Password[:], []byte("password-test"))
	var buffer bytes.Buffer
	if err := binary.Write(&buffer, binary.BigEndian, connInfo); err != nil {
		log.Error(err)
		return
	}
	log.Infof("SendToServer,idFrom:%d,idTo:%d", idFrom, idTo)

	return util.SendWithLock(lockSend, conn, buffer.Bytes(), connType)
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

	//连接AUX，以确定CLient NAT类型
	if err := SendConnectAUX(util.CfgNet.Proto, (*conn).LocalAddr().String(), util.CfgNet.ServerIP, 8088); err != nil {
		log.Debug(err)
		return
	}

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
			x = false
			id, err := strconv.ParseInt(params[1], 10, 64)
			if err != nil {
				log.Error(err)
				continue
			}
			if err := sendConnect(conn, ClienID, id, util.CONNECT_SRC); err != nil {
				return
			}

		case "to":
		}
	}
}

func SendConnectAUX(proto string, addr string, addrTo string, portTo int) (errRet error) {
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

	var buffer bytes.Buffer
	if err := binary.Write(&buffer, binary.BigEndian, ClienID); err != nil {
		log.Error(err)
		return
	}
	log.Infof("SendAUX ToServer :8088")
	//这里可以连接8088端口，，，aux
	errRet = util.Send(conn, buffer.Bytes(), util.CONNECT_AUX)

	(*conn).Close()
	return
}
