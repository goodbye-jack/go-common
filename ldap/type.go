package ldap

import (
	"context"
)

type User struct {
	Name string `json:"name"`
}

type Group struct {
	Name string `json:"name"`
}


type interface Ldap {
	AddUser(ctx context.Context, u *User) error
	UpdateUser(ctx context.Context, u *User) error
	DeleteUser(ctx context.Context, u *User) error

	ListUser(ctx context.Context) ([]*User, error)

	AddGroup(ctx context.Context, g *Group) error
	UpdateGroup(ctx context.Context, g *Group) error
	DeleteGroup(ctx context.Context, g *Group) error

	ListGroup(ctx context.Context) ([]*Group, error)
}
