package controller

import (
	"context"

	"github.com/shockerjue/gffg/example/protocol"

	"go.uber.org/zap"

	"github.com/shockerjue/gffg/zzlog"
)

func (this *controller) CreateUser(ctx context.Context, req *protocol.CreateUserReq) (resp *protocol.CreateUserResp, err error) {
	resp = &protocol.CreateUserResp{
		Code:  200,
		Msg:   "CreateUser Success " + req.Username,
		Extra: make(map[string]string),
	}

	zzlog.Infow("CreateUser success", zap.String("Username", req.Username), zap.Any("email", req.Email))
	return
}

func (this *controller) UserInfo(ctx context.Context, req *protocol.UserInfoReq) (resp *protocol.UserInfoResp, err error) {
	resp = &protocol.UserInfoResp{
		Code:  200,
		Msg:   "UserInfo Success " + req.Username,
		Extra: make(map[string]string),
	}

	zzlog.Infow("userInfo called", zap.Any("resp", resp))
	return
}
