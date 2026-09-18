// open-voip 进程入口：解析启动参数并委托 bootstrap 组装运行时。
package main

import (
	"flag"
	"fmt"
	"os"

	"open-voip/internal/app"
)

func main() {
	configPath := flag.String("config", "", "配置文件路径（YAML，唯一配置源）")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "必须指定 -config，例如：open-voip -config /opt/open-voip/config.yml")
		os.Exit(2)
	}

	if err := app.Run(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
