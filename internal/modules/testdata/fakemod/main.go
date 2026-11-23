// Command fakemod is a test module whose behaviour is chosen by FAKEMOD_MODE.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/alekzonder/onecheck/pkg/module"
)

func main() {
	mode := os.Getenv("FAKEMOD_MODE")
	if len(os.Args) > 1 && os.Args[1] == "describe" {
		switch mode {
		case "describe-garbage":
			fmt.Println("this is not json")
			return
		case "describe-fail":
			os.Exit(3)
		}
	}
	if len(os.Args) > 1 && os.Args[1] == "check" {
		switch mode {
		case "exit3":
			fmt.Fprintln(os.Stderr, "boom on stderr")
			os.Exit(3)
		case "garbage":
			fmt.Println("{not json")
			return
		case "hang":
			child := exec.Command("sleep", "60")
			if err := child.Start(); err != nil {
				panic(err)
			}
			_ = os.WriteFile(os.Getenv("FAKEMOD_PIDFILE"), []byte(strconv.Itoa(child.Process.Pid)), 0o644)
			time.Sleep(60 * time.Second)
		case "huge":
			buf := make([]byte, 1<<20)
			for i := range buf {
				buf[i] = 'a'
			}
			for i := 0; i < 64; i++ {
				_, _ = os.Stdout.Write(buf)
			}
			return
		}
	}
	protocol := module.ProtocolVersion
	if mode == "protocol99" {
		protocol = 99
	}
	m := module.Module{
		Description: module.Description{
			Name:        "fake",
			Version:     "1.2.3",
			Description: "fake module for tests",
			Fields:      map[string]module.FieldInfo{"echo": {Type: module.TypeString, Description: "the rule name"}},
		},
		Collect: func(_ context.Context, req module.Request) (map[string]any, error) {
			fmt.Fprintln(os.Stderr, "collecting for", req.Rule)
			return map[string]any{"echo": req.Rule, "timeout_ms": req.TimeoutMS}, nil
		},
	}
	if protocol != module.ProtocolVersion && len(os.Args) > 1 && os.Args[1] == "describe" {
		fmt.Printf(`{"protocol":%d,"name":"fake","version":"9.0.0","fields":{}}`+"\n", protocol)
		return
	}
	module.Main(m)
}
