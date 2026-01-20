package ldap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/utils"
	"strconv"
)

type LLDap struct {
	client         *http.HTTPClient
	service_name   string
	tenant         string
	admin          string
	admin_password string

	access_token  string
	refresh_token string
}

func NewLLDap(service_name, admin, admin_password string) Ldap {
	lldap := LLDap{
		service_name:   service_name,
		admin:          admin,
		admin_password: admin_password,
	}

	tenant := ""
	lldap.client = http.NewHTTPClient(tenant, service_name)

	if err := lldap.accessToken(); err != nil {
		log.Panic("%+v accessToken error, %v", lldap, err)
	}

	return &lldap
}

func (lldap *LLDap) accessToken() error {
	body := fmt.Sprintf(`{"username": "%s", "password":"%s"}`, lldap.admin, lldap.admin_password)
	headers := map[string]string{}

	ctx := context.Background()
	resp, err := lldap.client.Post(ctx, utils.LLDapLoginURL, []byte(body), headers)
	if err != nil {
		return err
	}

	respToken := struct {
		Token        string `json:"token"`
		RefreshToken string `json:"refreshToken"`
	}{}

	if err := json.Unmarshal(resp, &respToken); err != nil {
		return err
	}

	lldap.access_token = respToken.Token
	lldap.refresh_token = respToken.RefreshToken
	log.Info("accessToken success, %+v", lldap)

	return nil
}

func (lldap *LLDap) refreshToken(ctx context.Context) error {
	body := []byte{}
	headers := map[string]string{
		"Cookie": fmt.Sprintf("refresh_token=%s", lldap.refresh_token),
	}

	resp, err := lldap.client.Get(ctx, utils.LLDapRefreshTokenURL, body, headers)
	if err != nil {
		log.Info("refresh_token=%s", lldap.refresh_token)
		log.Error("refreshToken/Get(%s) error, %v", utils.LLDapRefreshTokenURL, err)
		return err
	}

	token := struct {
		Token string `json:"token"`
	}{}

	if err := json.Unmarshal(resp, &token); err != nil {
		log.Error("refreshToken/Unmarshal(%s), error, %v", string(resp), err)
		return err
	}

	lldap.access_token = token.Token

	return nil
}

func (lldap LLDap) getTenant(ctx context.Context) string {
	return ctx.Value(utils.TenantContextName).(string)
}

func (lldap LLDap) doGraphQL(ctx context.Context, query string, variables interface{}) ([]byte, error) {
	if err := lldap.refreshToken(ctx); err != nil {
		log.Error("doGraphQL/refreshToken error, %v", err)
		return nil, err
	}

	qv := struct {
		Query     string      `json:"query"`
		Variables interface{} `json:"variables"`
	}{
		Query:     query,
		Variables: variables,
	}

	body, err := json.Marshal(qv)
	if err != nil {
		log.Error("doGraphQL/Marshal(%+v), error, %v", qv, err)
		return nil, err
	}
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", lldap.access_token),
	}

	return lldap.client.Post(ctx, utils.LLDapGraphURL, body, headers)
}

func (lldap LLDap) respGraphQL(ctx context.Context, resp []byte, data interface{}) error {
	type Error struct {
		Message string `json:"message"`
	}
	obj := struct {
		Data   json.RawMessage `json:"data"`
		Errors []*Error        `json:"errors"`
	}{}
	if err := json.Unmarshal(resp, &obj); err != nil {
		log.Error("respGraphQL/Unmarshal error, %v", err)
		return err
	}

	if obj.Data == nil {
		return LdapIntervalError{}
	}

	if err := json.Unmarshal(obj.Data, data); err != nil {
		log.Error("respGraphQL/Unmarshal2 error, %v", err)
		return err
	}

	if len(obj.Errors) > 0 {
		return errors.New(obj.Errors[0].Message)
	}
	return nil
}

func (lldap LLDap) GetUser(ctx context.Context, id string) (*User, error) {
	query := `query user($id:String!){user(userId:$id){id email displayName firstName lastName avatar groups{id uuid displayName}}}`
	variables := struct {
		ID string `json:"id"`
	}{
		ID: id,
	}

	resp, err := lldap.doGraphQL(ctx, query, variables)
	if err != nil {
		log.Error("GetUser/doGraphQL(%s, %s) error, %v", query, variables, err)
		return nil, err
	}

	data := struct {
		User *User `json:"user"`
	}{}
	if err := lldap.respGraphQL(ctx, resp, &data); err != nil {
		log.Error("GetUser/respGraphQL error, %v", err)
		return nil, err
	}
	return data.User, nil
}

func (lldap LLDap) AddUser(ctx context.Context, u *User) error {
	query := `mutation createUser($user:CreateUserInput!){createUser(user:$user){id email displayName firstName lastName avatar}}`
	variables := struct {
		User *User `json:"user"`
	}{
		User: u,
	}

	resp, err := lldap.doGraphQL(ctx, query, variables)
	if err != nil {
		log.Error("AddUser/doGraphQL(%s, %s) error, %v", query, variables, err)
		return err
	}

	data := struct {
		CreateUser *User `json:"createUser"`
	}{}
	if err := lldap.respGraphQL(ctx, resp, &data); err != nil {
		log.Error("AddUser/respGraphQL error, %v", err)
		return err
	}
	log.Info("AddUser resp data = %+v", data)

	return nil
}

func (lldap LLDap) UpdateUser(ctx context.Context, u *User) error {
	query := `mutation updateUser($user:UpdateUserInput!){updateUser(user:$user){ok}}`
	type updateUserForm struct {
		ID          string `json:"id"`
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		FirstName   string `json:"firstName"`
		LastName    string `json:"lastName"`
		Avatar      string `json:"avatar"`
	}
	variables := struct {
		User *updateUserForm `json:"user"`
	}{
		User: &updateUserForm{
			ID:          u.ID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			FirstName:   u.FirstName,
			LastName:    u.LastName,
			Avatar:      u.Avatar,
		},
	}

	resp, err := lldap.doGraphQL(ctx, query, variables)
	if err != nil {
		log.Error("UpdateUser/doGraphQL(%s, %s) error, %v", query, variables, err)
		return err
	}

	type UpdateUser struct {
		Ok bool `json:"ok"`
	}
	data := struct {
		UpdateUser *UpdateUser `json:"updateUser"`
	}{}
	if err := lldap.respGraphQL(ctx, resp, &data); err != nil {
		log.Error("UpdateUser/respGraphQL error, %v", err)
		return err
	}

	if data.UpdateUser != nil && !data.UpdateUser.Ok {
		return LdapUpdateError{
			Type: "User",
			ID:   u.ID,
		}
	}
	log.Info("UpdateUser resp data = %+v", data.UpdateUser)
	return nil
}

func (lldap LLDap) DeleteUser(ctx context.Context, u *User) error {
	query := `mutation deleteUser($userId:String!){deleteUser(userId:$userId){ok}}`
	variables := struct {
		UserId string `json:"userId"`
	}{
		UserId: u.ID,
	}

	resp, err := lldap.doGraphQL(ctx, query, variables)
	if err != nil {
		log.Error("DeleteUser/doGraphQL(%s, %s) error, %v", query, variables, err)
		return err
	}

	type DeleteUser struct {
		Ok bool `json:"ok"`
	}
	data := struct {
		DeleteUser *DeleteUser `json:"deleteUser"`
	}{}
	if err := lldap.respGraphQL(ctx, resp, &data); err != nil {
		log.Error("DeleteUser/respGraphQL error, %v", err)
		return err
	}

	log.Info("DeleteUser resp data = %+v", data.DeleteUser)
	if data.DeleteUser != nil && !data.DeleteUser.Ok {
		return LdapDeleteError{
			Type: "User",
			ID:   u.ID,
		}
	}
	return nil
}

func (lldap LLDap) getGroupByName(ctx context.Context, displayName string) (*Group, error) {
	gs, err := lldap.ListGroup(ctx)
	if err != nil {
		return nil, err
	}
	for _, g := range gs {
		if g.DisplayName == displayName {
			return g, nil
		}
	}
	return nil, nil
}

func (lldap LLDap) GetGroup(ctx context.Context, id string) (*Group, error) {
	_id, err := strconv.Atoi(id)
	if err != nil {
		return nil, LdapParamsError{
			Params: []string{"id"},
		}
	}
	log.Info("GetGroup(%s)", id)

	query := `query group($id:Int!){group(groupId:$id){id uuid displayName users{id email displayName}}}`
	variables := struct {
		ID int `json:"id"`
	}{
		ID: _id,
	}

	resp, err := lldap.doGraphQL(ctx, query, variables)
	if err != nil {
		log.Error("GetGroup/doGraphQL(%s, %s) error, %v", query, variables, err)
		return nil, err
	}

	data := struct {
		Group *Group `json:"group"`
	}{}
	if err := lldap.respGraphQL(ctx, resp, &data); err != nil {
		log.Error("AddUser/respGraphQL error, %v", err)
		return nil, err
	}
	log.Info("GetGroup(%s), %v", id, data.Group)
	return data.Group, nil
}

func (lldap LLDap) AddGroup(ctx context.Context, g *Group) error {
	if _g, _ := lldap.getGroupByName(ctx, g.DisplayName); _g != nil {
		g.ID = _g.ID
		return LdapDuplicateError{}
	}

	query := `mutation createGroup($group:String!){createGroup(name:$group){id}}`
	variables := struct {
		Group string `json:"group"`
	}{
		Group: g.DisplayName,
	}

	resp, err := lldap.doGraphQL(ctx, query, variables)
	if err != nil {
		log.Error("AddGroup/doGraphQL(%s, %s) error, %v", query, variables, err)
		return err
	}

	data := struct {
		CreateGroup *Group `json:"createGroup"`
	}{}
	if err := lldap.respGraphQL(ctx, resp, &data); err != nil {
		log.Error("AddGroup/respGraphQL error, %v", err)
		return err
	}
	log.Info("AddGroup resp data = %+v", data.CreateGroup)

	g.ID = data.CreateGroup.ID

	return nil
}

func (lldap LLDap) UpdateGroup(ctx context.Context, g *Group) error {
	if g == nil {
		return LdapParamsError{
			Params: []string{"group.ID"},
		}
	}
	if _g, _ := lldap.getGroupByName(ctx, g.DisplayName); _g != nil {
		return LdapDuplicateError{}
	}

	type UpdateGroupForm struct {
		ID          int    `json:"id"`
		DisplayName string `json:"displayName"`
	}

	query := "mutation updateGroup($group:UpdateGroupInput!){updateGroup(group:$group){ok}}"
	variables := struct {
		Group *UpdateGroupForm `json:"group"`
	}{
		Group: &UpdateGroupForm{
			ID:          g.ID,
			DisplayName: g.DisplayName,
		},
	}

	resp, err := lldap.doGraphQL(ctx, query, variables)
	if err != nil {
		log.Error("AddGroup/doGraphQL(%s, %s) error, %v", query, variables, err)
		return err
	}

	type UpdateGroup struct {
		Ok bool `json:"ok"`
	}

	data := struct {
		UpdateGroup *UpdateGroup `json: "updateGroup"`
	}{}
	if err := lldap.respGraphQL(ctx, resp, &data); err != nil {
		log.Error("UpdateGroup/respGraphQL error, %v", err)
		return err
	}
	if data.UpdateGroup != nil && !data.UpdateGroup.Ok {
		return LdapUpdateError{
			Type: "Group",
			ID:   g.ID,
		}
	}
	log.Info("UpdateGroup(%v) return %v", g, data.UpdateGroup)
	return nil
}

func (lldap LLDap) DeleteGroup(ctx context.Context, g *Group) error {
	if g == nil {
		return LdapParamsError{
			Params: []string{"g"},
		}
	}

	query := "mutation deleteGroup($id:Int!){deleteGroup(groupId:$id){ok}}"
	variables := struct {
		ID int `json:"id"`
	}{
		ID: g.ID,
	}

	resp, err := lldap.doGraphQL(ctx, query, variables)
	if err != nil {
		log.Error("DeleteGroup/doGraphQL(%s, %s) error, %v", query, variables, err)
		return err
	}

	type DeleteGroup struct {
		Ok bool `json:"ok"`
	}
	data := struct {
		DeleteGroup *DeleteGroup `json:"deleteGroup"`
	}{}
	if err := lldap.respGraphQL(ctx, resp, &data); err != nil {
		log.Error("DeleteGroup/respGraphQL error, %v", err)
		return err
	}
	if data.DeleteGroup != nil && !data.DeleteGroup.Ok {
		return LdapDeleteError{
			Type: "Group",
			ID:   g.ID,
		}
	}
	log.Info("DeleteGroup(%+v) return %v", g, data.DeleteGroup)
	return nil
}

func (lldap LLDap) ListUser(ctx context.Context) ([]*User, error) {
	query := `{users{id creationDate uuid email displayName firstName lastName groups{id uuid displayName}}}`
	variables := ""
	resp, err := lldap.doGraphQL(ctx, query, variables)
	if err != nil {
		log.Error("ListUser/doGraphQL(%s, %s), error, %v", query, variables, err)
		return nil, err
	}

	data := struct {
		Users []*User `json:"users"`
	}{}
	if err := lldap.respGraphQL(ctx, resp, &data); err != nil {
		log.Error("ListUser/respGraphQL error, %v", err)
		return nil, err
	}

	return data.Users, nil
}

func (lldap LLDap) ListGroup(ctx context.Context) ([]*Group, error) {
	query := `{groups{id creationDate uuid displayName users{id email displayName}}}`
	variables := ""
	resp, err := lldap.doGraphQL(ctx, query, variables)
	if err != nil {
		log.Error("ListGroup/doGraphQL(%s, %s), error, %v", query, variables, err)
		return nil, err
	}

	data := struct {
		Groups []*Group `json:"groups"`
	}{}
	if err := lldap.respGraphQL(ctx, resp, &data); err != nil {
		log.Error("ListUser/respGraphQL error, %v", err)
		return nil, err
	}

	return data.Groups, nil
}

func (lldap LLDap) JoinGroup(ctx context.Context, u *User, g *Group) error {
	if u == nil || g == nil {
		return errors.New("u || g is empty")
	}

	query := "mutation addUserToGroup($userId: String!, $groupId: Int!){addUserToGroup(userId:$userId,groupId:$groupId){ok}}"
	variables := struct {
		UserId  string `json:"userId"`
		GroupId int    `json:"groupId"`
	}{
		UserId:  u.ID,
		GroupId: g.ID,
	}
	resp, err := lldap.doGraphQL(ctx, query, variables)
	if err != nil {
		log.Error("JoinGroup/doGraphQL(%s, %s) error, %v", query, variables, err)
		return err
	}

	type addUserToGroup struct {
		Ok bool `json:"ok"`
	}
	data := struct {
		addUserToGroup *addUserToGroup `json:"addUserToGroup"`
	}{}
	if err := lldap.respGraphQL(ctx, resp, &data); err != nil {
		log.Error("JoinGroup/respGraphQL error, %v", err)
		return err
	}

	if data.addUserToGroup != nil && !data.addUserToGroup.Ok {
		return LdapIntervalError{}
	}

	log.Info("JoinGroup(%+v, %+v) success", u, g)
	return nil
}

func (lldap LLDap) QuitGroup(ctx context.Context, u *User, g *Group) error {
	if u == nil || g == nil {
		return errors.New("u || g is empty")
	}

	query := "mutation removeUserFromGroup($userId: String!, $groupId: Int!){removeUserFromGroup(userId:$userId,groupId:$groupId){ok}}"
	variables := struct {
		UserId  string `json:"userId"`
		GroupId int    `json:"groupId"`
	}{
		UserId:  u.ID,
		GroupId: g.ID,
	}
	resp, err := lldap.doGraphQL(ctx, query, variables)
	if err != nil {
		log.Error("QuitGroup/doGraphQL(%s, %s) error, %v", query, variables, err)
		return err
	}

	type removeUserFromGroup struct {
		Ok bool `json:"ok"`
	}
	data := struct {
		removeUserFromGroup *removeUserFromGroup `json:"removeUserFromGroup"`
	}{}
	if err := lldap.respGraphQL(ctx, resp, &data); err != nil {
		log.Error("JoinGroup/respGraphQL error, %v", err)
		return err
	}

	if data.removeUserFromGroup != nil && !data.removeUserFromGroup.Ok {
		return LdapIntervalError{}
	}

	log.Info("QuitGroup(%+v, %+v) success", u, g)
	return nil
}
