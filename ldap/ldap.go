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

type User struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	DisplayName string   `json:"displayName"`
	FirstName   string   `json:"firstName"`
	LastName    string   `json:"lastName"`
	Avatar      string   `json:"avatar"`
	Groups      []Group `json:"groups,omitempty"`
}

type Group struct {
	ID          int  `json:"id"`
	UUID        string  `json:"uuid"`
	DisplayName string  `json:"displayName"`
	Users      []User `json:"users,omitempty"`
}

type Ldap interface {
	GetUser(ctx context.Context, id string) (*User, error)
	AddUser(ctx context.Context, u *User) error
	UpdateUser(ctx context.Context, u *User) error
	DeleteUser(ctx context.Context, u *User) error

	ListUser(ctx context.Context) ([]*User, error)

	GetGroup(ctx context.Context, id string) (*Group, error)
	AddGroup(ctx context.Context, g *Group) error
	UpdateGroup(ctx context.Context, g *Group) error
	DeleteGroup(ctx context.Context, g *Group) error

	ListGroup(ctx context.Context) ([]*Group, error)

	JoinGroup(ctx context.Context, u *User, g *Group) error
	QuitGroup(ctx context.Context, u *User, g *Group) error
}
