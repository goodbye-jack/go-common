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

	u.DisplayName = "Han RedHan"

	lldap.UpdateUser(ctx, u)

	lldap.DeleteUser(ctx, u)
	lldap.DeleteUser(ctx, u)

	users, err := lldap.ListUser(ctx)
	if err != nil {
		fmt.Printf("ListUser, error, %v", err)
	}
	fmt.Printf("%+v", users)

	g := Group {
		DisplayName: "xxxLeague",
	}

	if err := lldap.AddGroup(ctx, &g); err != nil {
		g.DisplayName = "newLeague"
		lldap.UpdateGroup(ctx, &g)
		lldap.DeleteGroup(ctx, &g)
	}

	groups, err := lldap.ListGroup(ctx)
	if err != nil {
		fmt.Println("error")
	}

	for _, group := range groups {
		fmt.Printf("%d", group.ID)
	}
}
