package dao

import (
	"net"
	"sync"
	//	"sync/atomic"
	"errors"
	"time"

	"github.com/lxt1045/TCPHolePunching/util"
)

var log util.MylogStruct
var lockSend *sync.Mutex
var connClient *net.Conn
var local *time.Location

//生成两个索引
var currentID int64

type ConnStruct struct {
	Conn       *net.Conn
	ID         int64
	Addr       string
	Lock       *sync.Mutex
	ClientType int32 //0： 默认类型； 1：处于外网；2：IP：Port相同则Nat映射端口相同；3:不同不能连接打通
}

var connMapID map[int64]*ConnStruct
var connMapAddr map[string]*ConnStruct

func init() {
	connMapID = make(map[int64]*ConnStruct)
	connMapAddr = make(map[string]*ConnStruct)
}

func Add(addr string, id int64, conn *net.Conn) (connRet *ConnStruct, err error) {
	if _, ok := connMapID[id]; ok {
		err = errors.New("连接ID编号已存在")
		util.Mylog.Debug("连接ID编号已存在！")
	}
	if val, ok := connMapAddr[addr]; ok {
		connRet = val
		util.Mylog.Debug("连接地址已存在！")
	}

	connRet = &ConnStruct{
		Conn: conn,
		ID:   id,
		Addr: addr,
		Lock: new(sync.Mutex),
	}
	connMapID[id] = connRet
	connMapAddr[addr] = connRet
	util.Mylog.Debug(connRet)

	return
}
func Remove(id int64) {
	conn, ok := connMapID[id]
	if !ok {
		util.Mylog.Debug("连接ID编号不存在！")
		return
	}
	util.Mylog.Debug("删除：", *conn)
	conn.Lock.Lock()
	delete(connMapID, id)
	delete(connMapAddr, conn.Addr)
	conn.Lock.Unlock()
	return
}

func GetByID(id int64) (connRet *ConnStruct, okRet bool) {
	if v, ok := connMapID[id]; ok {
		connRet = v
		okRet = true
		return
	} else {
		util.Mylog.Debug("没找到,V:", v)
	}

	okRet = false

	return
}

func GetByAddr(addr string) (connRet *ConnStruct, okRet bool) {
	if val, ok := connMapAddr[addr]; ok {
		connRet = val
		return
	}

	util.Mylog.Debug("没找到")
	okRet = false

	return
}
