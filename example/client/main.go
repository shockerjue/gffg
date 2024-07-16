package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"

	"go.uber.org/zap"

	"github.com/shockerjue/gffg/example/protocol"

	"github.com/shockerjue/gffg/client"
	"github.com/shockerjue/gffg/config"
	"github.com/shockerjue/gffg/zzlog"
)

func createUser(c *client.Client, i int) {
	req := &protocol.CreateUserReq{
		Auth:      &protocol.Authorize{Appid: "", Appkey: ""},
		Username:  fmt.Sprintf("%s-%d", "gffg", i),
		Telephone: "1234567890",
		Email:     string("HelloWorld"),
	}
	userService := protocol.NewUserService(c, "gffg-test")
	resp, err := userService.CreateUser(context.TODO(), req, client.Timeout(5))
	if nil != err {
		zzlog.Errorw("CreateUser return error", zap.Error(err))
	} else {
		if "CreateUser Success "+req.Username != resp.Msg {
			zzlog.Errorw("Reuqest is not equal response ", zap.Any("request", req.Username), zap.Any("response", resp.Msg))
		} else {
			zzlog.Infow("Create return", zap.Any("resp", resp), zap.Error(err))
		}
	}
}

func userInfo(c *client.Client) {
	userService := protocol.NewUserService(c, "gffg-test")

	// Calling an interface without responding to gffg-test server
	// Set timeout 5s
	_, err := userService.UserInfo(context.TODO(), &protocol.UserInfoReq{
		Auth:     &protocol.Authorize{Appid: "", Appkey: ""},
		Username: fmt.Sprintf("%s", "gffg"),
	}, client.OnlyCall(true), client.Timeout(5))
	if nil != err {
		zzlog.Errorw("UserInfo call error", zap.Error(err))
	}
}

func main() {
	config.Init("./conf/gffg.xml")

	zzlog.Init(
		zzlog.WithLogName(config.Get("log", "log_file").String("")),
		zzlog.WithLevel(config.Get("log", "level").String("info")))

	c := client.NewClient(config.Get("client", "group").String("basesvr"))
	defer c.Destroy()

	go func() {
		var reqs int64
		var wg sync.WaitGroup
		for {
			for i := 0; i < 50; i++ {
				wg.Add(2)
				go func(i int) {
					defer wg.Done()
					createUser(c, i)
				}(i)
				go func() {
					defer wg.Done()

					userInfo(c)
				}()
			}
			wg.Wait()
			reqs += 100

			zzlog.Warnw("goruntine.Num ------------- ", zap.Any("", runtime.NumGoroutine()), zap.Any("reqs", reqs))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}
