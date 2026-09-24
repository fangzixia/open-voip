// open-switch 进程入口。
package main

import (
	"flag"
	"fmt"
	"os"

	"open-switch/internal/app"
)

func main() {
	configPath := flag.String("config", "", "配置文件路径（YAML）")
	flag.Parse()
	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "必须指定 -config，例如：open-switch -config deploy/config.example.yml")
		os.Exit(2)
	}
	if err := app.Run(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
