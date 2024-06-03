package ldap

type LLDap struct {
	url string
	admin string
	admin_password string
}

func NewLLDap(url, admin, admin_password) Ldap {
	return &LLDap {
		url: url,
		admin: admin,
		admin_password: admin_password,
	}
}

func (lld *LLDap) AddUser(u *User) error {
	return nil
}

func (lld *LLDap) UpdateUser(u *User) error {
	return nil
}

func (lld *LLDap) DeleteUser(u *User) error {
	return nil
}

func (lld *LLDap) AddGroup(g *Group) error {
	return nil
}

func (lld *LLDap) UpdateGroup(g *Group) error {
	return nil
}

func (lld *LLDap) Delete(g *Group) error {
	return nil
}

func (lld *LLDap) ListUser() ([]*User, error) {
	return nil, nil
}

func (lld *LLDap) ListGroup() ([]*Group, error) {
	return nil, nil
}
