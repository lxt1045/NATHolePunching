package util

import (
	"flag"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/cihub/seelog"
)

const (
	HEART_BEAT = 1 //心跳包：C-->S

	//Client通知Server需要向某个ID发起连接：C-->S
	//Server 通知 被连接Client，有Client想要连接你，请尝试"铺路"：S-->C
	CONNECT = 2

	//通知Server，Client需要获取自己的ID：C-->S
	//通知Client，这是你的ID：S-->C
	ID = 3
)

func init() {
	sections, meta := initConf()
	initLog(sections, meta)
}

func initLog(sections map[string]toml.Primitive, meta toml.MetaData) {
	// 解析服务配置文件
	xml := `
	<seelog>
    <outputs formatid="main">
        <filter levels="info,critical,error,debug">
            <console formatid="main" />
            <rollingfile formatid="main" type="date" filename="./seelog/debug.seelog" datepattern="2006.01.02" />
        </filter>
    </outputs>

    <formats>
        <format id="main" format="%Date %Time [%LEV] %Msg [%File][%FuncShort][%Line]%n"/>
    </formats>
	</seelog>
	`
	var cfg struct {
		File string `toml:"file"` // 日志配置文件的路径
	}

	// 加载配置文件中的值
	var sectionName = "seelog"
	if section, ok := sections[sectionName]; ok {
		if err := meta.PrimitiveDecode(section, &cfg); err != nil {
			seelog.Error("配置文件出错：", err)
			return
		}
	}

	if cfg.File == "" {
		// 解析日志配置（从默认配置）
		logger, err := seelog.LoggerFromConfigAsBytes([]byte(xml))
		if err != nil {
			panic(fmt.Errorf("seelog configuration parse error: %s", err.Error()))
		}
		seelog.ReplaceLogger(logger)
	} else {
		// 解析日志配置
		logger, err := seelog.LoggerFromConfigAsFile(cfg.File)
		if err != nil {
			panic(fmt.Errorf("seelog configuration parse error: %s", err.Error()))
		}
		seelog.ReplaceLogger(logger)
	}
}

// init 包初始化
func initConf() (sections map[string]toml.Primitive, meta toml.MetaData) {
	// 用于记录服务配置信息的变量
	var file toml.Primitive
	//var sections map[string]toml.Primitive
	//var meta toml.MetaData

	// 配置文件名称
	fileName := "config.conf"

	// 解析命令行参数
	flagFile := flag.String("conf", "", "configuration file Name")
	flag.Parse()
	if *flagFile != "" {
		fileName = *flagFile
	}

	// 判断配置文件是否存在
	if _, err := os.Stat(fileName); err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("configuration file %s does not exist.\r\n", fileName)
		} else {
			fmt.Printf("configuration file %s execption:%s\r\n", fileName, err.Error())
		}
		fileName = ""
		return
	}

	// 加载配置文件
	if fileName != "" {
		var err error
		if meta, err = toml.DecodeFile(fileName, &file); err != nil {
			panic(fmt.Errorf("load configuration file %s failed:%s", fileName, err.Error()))
		}
		if err = meta.PrimitiveDecode(file, &sections); err != nil {
			panic(fmt.Errorf("load configuration file %s failed:%s", fileName, err.Error()))
		}
	}
	fmt.Printf("load configuration file %s succeed.\r\n", fileName)

	return
}

type MylogStruct struct {
	Tracef, Debugf, Infof    func(format string, params ...interface{})
	Warnf, Errorf, Criticalf func(format string, params ...interface{}) error
	Trace, Debug, Info       func(v ...interface{})
	Warn, Error, Critical    func(v ...interface{}) error

	// Flush 将所有日志信息写入缓存
	Flush func()
}

var Mylog = MylogStruct{
	seelog.Tracef, seelog.Debugf, seelog.Infof,
	seelog.Warnf, seelog.Errorf, seelog.Criticalf,
	seelog.Trace, seelog.Debug, seelog.Info,
	seelog.Warn, seelog.Error, seelog.Critical,
	// Flush 将所有日志信息写入缓存
	seelog.Flush,
}
