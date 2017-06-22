//+build linux
package util

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func init() {

}

func Socket0(proto, addr string) (l net.Listener, err error) {
	var (
		fd   int
		file *os.File
	)

	if "tcp" != proto {
		Mylog.Error("tcp != proto")
		return
	}
	tcp, err := net.ResolveTCPAddr(proto, addr)
	if err != nil && tcp.IP != nil {
		Mylog.Error(err)
		return
	}
	sockaddr, soType := &syscall.SockaddrInet4{Port: tcp.Port}, syscall.AF_INET

	syscall.ForkLock.RLock()
	if fd, err = syscall.Socket(soType, syscall.SOCK_STREAM, syscall.IPPROTO_TCP); err != nil {
		syscall.ForkLock.RUnlock()
		return
	}
	syscall.ForkLock.RUnlock()

	defer func() {
		if err != nil {
			syscall.Close(fd)
		}
	}()

	if err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		return
	}

	const reusePort = 0x0F
	if err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, reusePort, 1); err != nil {
		return
	}

	if err = syscall.Bind(fd, sockaddr); err != nil {
		return
	}

	// Set backlog size to the maximum
	//是TCP模块允许的已完成三次握手过程(TCP模块完成)但还没来得及被应用程序accept的最大链接数
	if err = syscall.Listen(fd, 128); err != nil {
		return
	}

	getSocketFileName := func(proto, addr string) string {
		return fmt.Sprintf("reuseport.%d.%s.%s", os.Getpid(), proto, addr)
	}
	file = os.NewFile(uintptr(fd), getSocketFileName(proto, addr))
	if l, err = net.FileListener(file); err != nil {
		return
	}

	if err = file.Close(); err != nil {
		return
	}
	return
}

func Socket(proto, addr string) (fd int, err error) {

	if "tcp" != proto {
		Mylog.Error("tcp != proto")
		return
	}
	tcp, err := net.ResolveTCPAddr(proto, addr)
	if err != nil && tcp.IP != nil {
		Mylog.Error(err)
		return
	}
	sockaddr := &syscall.SockaddrInet4{Port: tcp.Port}

	syscall.ForkLock.RLock()
	if fd, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP); err != nil {
		syscall.ForkLock.RUnlock()
		return
	}
	syscall.ForkLock.RUnlock()

	defer func() {
		if err != nil {
			syscall.Close(fd)
		}
	}()

	if err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		return
	}

	const reusePort = 0x0F
	if err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, reusePort, 1); err != nil {
		return
	}

	if err = syscall.Bind(fd, sockaddr); err != nil {
		return
	}

	return
}

func Listen(fd int, fun func(*net.Conn) error) (err error) {
	// Set backlog size to the maximum
	//是TCP模块允许的已完成三次握手过程(TCP模块完成)但还没来得及被应用程序accept的最大链接数
	if err = syscall.Listen(fd, 128); err != nil {
		Mylog.Debug("Listen err:", err)
		return
	}

	//	getSocketFileName := func(proto, addr string) string {
	//		return fmt.Sprintf("reuseport.%d.%s.%s", os.Getpid(), proto, addr)
	//	}

	var file *os.File
	var l net.Listener
	file = os.NewFile(uintptr(fd), fmt.Sprintf("tcpholepunching.%d", time.Now().UnixNano()))
	if l, err = net.FileListener(file); err != nil {
		return
	}

	if err = file.Close(); err != nil {
		return
	}
	defer l.Close()

	var tempDelay time.Duration // how long to sleep on accept failure
	for {
		Mylog.Debug("before Accept")
		rw, e := l.Accept()
		Mylog.Debug("after Accept")
		if e != nil {
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				Mylog.Infof("http: Accept error: %v; retrying in %v", e, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			err = e
			return
		}

		Mylog.Debugf("after Accept,SRC IP:%s, TGT IP:%s", rw.RemoteAddr().String(), rw.LocalAddr().String())

		tempDelay = 0
		go fun(&rw)
	}
}

func InetAddr(ipaddr string) [4]byte {
	var (
		ips = strings.Split(ipaddr, ".")
		ip  [4]uint64
		ret [4]byte
	)
	for i := 0; i < 4; i++ {
		ip[i], _ = strconv.ParseUint(ips[i], 10, 8)
	}
	for i := 0; i < 4; i++ {
		ret[i] = byte(ip[i])
	}
	return ret
}
func InetPort(ipport string) uint16 {
	ret, _ := strconv.ParseUint(ipport, 10, 16)
	return uint16(ret)
}

func Connect(fd int, addr [4]byte, port int) (conn *net.Conn, err error) {
	var addrInet4 syscall.SockaddrInet4
	addrInet4.Addr = addr //inetAddr(addr)
	addrInet4.Port = port
	Mylog.Debug("before Connect：", addrInet4)

	chConnect := make(chan error)
	go func() {
		err = syscall.Connect(fd, &addrInet4)
		chConnect <- err
	}()
	ticker := time.NewTicker(3 * time.Second)
	select {
	case <-ticker.C:
		err = fmt.Errorf("Connect timeout")
		Mylog.Error(err)
		return
	case e := <-chConnect:
		if e != nil {
			err = e
			Mylog.Error("Connect error: ", err)
			return
		}
	}

	//	if err = syscall.Connect(fd, &addrInet4); err != nil {
	//		Mylog.Error("Connect error: ", err)
	//		return
	//	}

	Mylog.Debug("after Connect")
	var file *os.File
	file = os.NewFile(uintptr(fd), fmt.Sprintf("tcpholepunching.%d", time.Now().UnixNano()))
	if conn0, err0 := net.FileConn(file); err != nil {
		Mylog.Error("Connect error", err0)
		err = err0
		return
	} else {
		conn = &conn0
	}

	if err = file.Close(); err != nil {
		Mylog.Error("Connect error", err)
		return
	}
	return
}

func ConnectScout(fd int, addr string, port int) (conn *net.Conn, err error) {
	inetAddr := func(ipaddr string) [4]byte {
		var (
			ips = strings.Split(ipaddr, ".")
			ip  [4]uint64
			ret [4]byte
		)
		for i := 0; i < 4; i++ {
			ip[i], _ = strconv.ParseUint(ips[i], 10, 8)
		}
		for i := 0; i < 4; i++ {
			ret[i] = byte(ip[i])
		}
		return ret
	}

	var addrInet4 syscall.SockaddrInet4
	addrInet4.Addr = inetAddr(addr)
	addrInet4.Port = port
	if err = syscall.Connect(fd, &addrInet4); err != nil {
		Mylog.Error("Connect error", err)
		return
	}

	var file *os.File
	file = os.NewFile(uintptr(fd), fmt.Sprintf("tcpholepunching.%d", time.Now().UnixNano()))
	if conn0, err0 := net.FileConn(file); err != nil {
		Mylog.Error("Connect error", err0)
		err = err0
		return
	} else {
		conn = &conn0
	}

	if err = file.Close(); err != nil {
		Mylog.Error("Connect error", err)
		return
	}
	return
}
