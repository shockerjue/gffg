package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/shockerjue/gffg/example/protocol"
	"github.com/shockerjue/gffg/example/server/controller"

	"github.com/shockerjue/gffg/server"
	"github.com/shockerjue/gffg/tools"
)

func main() {
	// Create a Server by config
	svr := server.NewServer("./conf/gffg.xml")
	defer svr.Release()

	// Register the interface implementation to the service
	ctl := controller.Controller()
	protocol.RegisterUserServiceHandler(svr, ctl)

	// Start rpc service
	svr.Run(server.Bind("0.0.0.0"), server.Port(0))

	// Enable performance detection service
	tools.PProf()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	return
}
