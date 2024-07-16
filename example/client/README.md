# client
You can request the interface provided by the rpc service through the following steps.

## config init
```
config.Init("./conf/gffg.xml")

zzlog.Init(
	zzlog.WithLogName(config.Get("log", "log_file").String("")),
	zzlog.WithLevel(config.Get("log", "level").String("info")))

```

## create client
Create a client object through the service group.

```
c := client.NewClient(config.Get("client", "group").String("basesvr"))
defer c.Destroy()
```

## call interface
Call the interface by specifying the service name.
```
req := &protocol.CreateUserReq{
		Username:  fmt.Sprintf("%s-%d", "gffg", i),
		Telephone: "1234567890",
		Email:     string("HelloWorld"),
	}
userService := protocol.NewUserService(c, "gffg-test")
resp, err := userService.CreateUser(context.TODO(), req, client.Timeout(5))
if nil != err {
	zzlog.Errorw("CreateUser return error", zap.Error(err))
}
```

You can also configure it to only request the service without requiring a service response.The configuration items are client.OnlyCall(true).
```
userService := protocol.NewUserService(c, "gffg-test")
_, err := userService.UserInfo(context.TODO(), &protocol.UserInfoReq{
  Username: fmt.Sprintf("%s", "gffg"),
}, client.OnlyCall(true))
if nil != err {
  zzlog.Errorw("UserInfo call error", zap.Error(err))
}
```
