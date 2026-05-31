package changeguard

type Guard interface {
	Name() string
	Check(*Session) error
}

type NoopGuard struct {
	name string
}

func NewNoopGuard(name string) *NoopGuard {
	return &NoopGuard{name: chooseNonEmpty(name, "noop")}
}

func (g *NoopGuard) Name() string {
	return g.name
}

func (g *NoopGuard) Check(*Session) error {
	return nil
}
