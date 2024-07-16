package registry

import (
	"context"

	"github.com/polarismesh/polaris-go/pkg/model"
)

type NodeInstance model.Instance

// Service Registry interface
type IRegistry interface {
	Provider(*node)
	// Subscribe server node
	Consumer()
	// Register the service node to the management center
	Register(string, string)
	Destroy()
	// Get node information based on service group and service name
	// @param 	ctx
	// @param	group 	Service Group Information
	// @param	name 	Service Name
	GetNode(context.Context, string, string) (NodeInstance, error)
	// Determine whether the service is restricted
	// @param	ctx
	// @param	name 	Server Name
	Limiter(context.Context, string) error
}
