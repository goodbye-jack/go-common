package ldap

import (
	"fmt"
	"testing"
	"context"
)

func TestNewLLDap(t *testing.T) {
	service_name := "lldap"
	admin := "admin"
	admin_password := "admin@1234"

	lldap := NewLLDap(service_name, admin, admin_password)

	u := &User {
		ID: "wenchao.han",
		DisplayName: "Han Wenchao",
		Email: "wenchao.han@gmail.com",
		FirstName: "Han",
		LastName: "Wenchao",
		Avatar: "",
	}
	ctx := context.Background()
	if err := lldap.AddUser(ctx, u); err != nil {
		fmt.Printf("%+v", err)
	}

	if _, err := lldap.GetUser(ctx, u.ID); err != nil {
		fmt.Printf("%v", err)
	}

	u.DisplayName = "Han RedHan"

	lldap.UpdateUser(ctx, u)

	g := &Group {
		DisplayName: "League",
	}
	if err := lldap.AddGroup(ctx, g); err == nil {

		lldap.GetGroup(ctx, g.DisplayName)

		lldap.JoinGroup(ctx, u, g)
		lldap.QuitGroup(ctx, u, g)

		g.DisplayName = "newLeague"
		lldap.UpdateGroup(ctx, g)
		lldap.DeleteGroup(ctx, g)
	}

	lldap.DeleteUser(ctx, u)

	users, err := lldap.ListUser(ctx)
	if err != nil {
		fmt.Printf("ListUser, error, %v", err)
	}
	fmt.Printf("%+v", users)

	groups, err := lldap.ListGroup(ctx)
	if err != nil {
		fmt.Println("error")
	}

	for _, group := range groups {
		fmt.Printf("%d", group.ID)
	}
}
