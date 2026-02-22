package ldap

import (
	"context"
	"fmt"
)

type LdapDuplicateError struct{}
type LdapIntervalError struct{}

func (e LdapIntervalError) Error() string {
	return fmt.Sprintf("Ldap internal error, %s", "")
}

func (e LdapDuplicateError) Error() string {
	return fmt.Sprintf("Ldap duplicate error, %s", "")
}

type LdapParamsError struct {
	Params []string
}

func (e LdapParamsError) Error() string {
	return fmt.Sprintf("params %+v error", e.Params)
}

type LdapUpdateError struct {
	ID   interface{}
	Type string
}

func (e LdapUpdateError) Error() string {
	return fmt.Sprintf("update %s(%s) error", e.Type, e.ID)
}

type LdapDeleteError struct {
	ID   interface{}
	Type string
}

func (e LdapDeleteError) Error() string {
	return fmt.Sprintf("delete %s(%s) error", e.Type, e.ID)
}

type LdapNotFoundError struct {
	ID   interface{}
	Type string
}

func (e LdapNotFoundError) Error() string {
	return fmt.Sprintf("%s(%s) not found", e.Type, e.ID)
}

type OrgUser struct {
	DN            string   `json:"dn"`
	UID           string   `json:"uid"`
	Password      string   `json:"password"`
	Phone         string   `json:"phone"`
	Address       string   `json:"address"`
	Gender        string   `json:"gender"`
	Birthday      string   `json:"birthday"`
	Email         string   `json:"email"`
	DisplayName   string   `json:"displayName"`
	FirstName     string   `json:"firstName"`
	LastName      string   `json:"lastName"`
	EmployeeNo    string   `json:"employeeNo"`
	Status        string   `json:"status"`
	DeptCodes     []string `json:"deptCodes"`
	PositionCodes []string `json:"positionCodes"`
}

type Department struct {
	DN        string `json:"dn"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	ParentDN  string `json:"parentDn"`
	ManagerDN string `json:"managerDn"`
	Status    string `json:"status"`
}

type Position struct {
	DN     string `json:"dn"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	DeptDN string `json:"deptDn"`
	Status string `json:"status"`
}

type Ldap interface {
	GetUser(ctx context.Context, uid string) (*OrgUser, error)
	AddUser(ctx context.Context, u *OrgUser) error
	UpdateUser(ctx context.Context, u *OrgUser) error
	DeleteUser(ctx context.Context, uid string) error
	ListUser(ctx context.Context) ([]*OrgUser, error)

	GetDepartment(ctx context.Context, code string) (*Department, error)
	AddDepartment(ctx context.Context, d *Department) error
	UpdateDepartment(ctx context.Context, d *Department) error
	DeleteDepartment(ctx context.Context, code string) error
	ListDepartment(ctx context.Context) ([]*Department, error)

	GetPosition(ctx context.Context, code string) (*Position, error)
	AddPosition(ctx context.Context, p *Position) error
	UpdatePosition(ctx context.Context, p *Position) error
	DeletePosition(ctx context.Context, code string) error
	ListPosition(ctx context.Context) ([]*Position, error)

	AddUserDepartments(ctx context.Context, uid string, deptCodes []string) error
	RemoveUserDepartments(ctx context.Context, uid string, deptCodes []string) error
	AddUserPositions(ctx context.Context, uid string, positionCodes []string) error
	RemoveUserPositions(ctx context.Context, uid string, positionCodes []string) error

	ValidateUser(ctx context.Context, phone, password string) (*OrgUser, error)
}
