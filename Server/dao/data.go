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
	Conn *net.Conn
	ID   int64
	Addr string
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
	}
	connMapID[id] = connRet
	connMapAddr[addr] = connRet

	return
}

func GetByID(id int64) (connRet *ConnStruct, okRet bool) {
	if v, ok := connMapID[id]; ok {
		connRet = v
		return
	}

	util.Mylog.Debug("没找到")
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
