package api

import (
	"github.com/goodbye-jack/go-common/workflow/assignment"
	"github.com/goodbye-jack/go-common/workflow/contract"
	"github.com/goodbye-jack/go-common/workflow/directory"
	"github.com/goodbye-jack/go-common/workflow/formref"
)

type RegisterOptions struct {
	DirectoryService  directory.Service
	AssignmentService assignment.Service
	FormRefService    formref.Service
	ContractPolicy    *contract.Policy
}
