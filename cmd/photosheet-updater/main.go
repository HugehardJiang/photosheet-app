package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"photosheet/internal/updater"
)

func main() {
	target := flag.String("target", "", "当前运行程序路径")
	update := flag.String("package", "", "已下载并校验的更新程序路径")
	backup := flag.String("backup", "", "旧程序备份路径")
	restart := flag.Bool("restart", true, "替换后重新启动程序")
	flag.Parse()
	if *target == "" || *update == "" || *backup == "" {
		fmt.Fprintln(os.Stderr, "用法：photosheet-updater --target <程序> --package <更新包> --backup <备份>")
		os.Exit(2)
	}
	var err error
	for attempt := 0; attempt < 30; attempt++ {
		err = updater.ApplyExecutable(*target, *update, *backup)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "更新失败：%v\n", err)
		os.Exit(1)
	}
	if *restart {
		if err := updater.Restart(*target); err != nil {
			fmt.Fprintf(os.Stderr, "更新完成但重启失败：%v\n", err)
			os.Exit(1)
		}
	}
}
