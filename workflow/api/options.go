package api

import (
	"context"

	"github.com/goodbye-jack/go-common/workflow/assignment"
	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/contract"
	"github.com/goodbye-jack/go-common/workflow/directory"
	"github.com/goodbye-jack/go-common/workflow/formref"
	"github.com/goodbye-jack/go-common/workflow/types"
)

// CallbackHandler 在验签和底座回调处理成功后同步执行；返回错误时不确认回调成功。
type CallbackHandler func(context.Context, *types.FlowableCallbackPayload) error

type RegisterOptions struct {
	DirectoryService  directory.Service
	AssignmentService assignment.Service
	ContextResolver   workflowcontext.Resolver
	FormRefService    formref.Service
	ContractPolicy    *contract.Policy
	CallbackHandler   CallbackHandler
}
