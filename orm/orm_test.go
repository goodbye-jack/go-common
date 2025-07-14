package orm

import (
	"context"
	"errors"
	"github.com/goodbye-jack/go-common/config"
	"gorm.io/gorm"
	"testing"
)

type Tester struct {
	gorm.Model

	Name string
}

func TestORM(t *testing.T) {
	orm := NewOrm(config.GetConfigString("tidb_dsn"), "mysql", 5)
	orm.AutoMigrate(&Tester{})
	tr := Tester{
		Name: "Tester",
	}
	ctx := context.Background()

	orm.Create(ctx, &tr)

	ntr := &Tester{}
	if err := orm.First(ctx, ntr, "name = ?", "Tester"); err != nil {
		t.Error(err)
	}
	t.Log(ntr.Name)

	orm.Transaction(ctx, func(tx *gorm.DB) error {
		tr2 := Tester{
			Name: "Tester3",
		}
		if err := tx.WithContext(ctx).Create(&tr2).Error; err != nil {
			return err
		}

		ntr := &Tester{}
		if err := orm.First(ctx, ntr, "name = ?", "Tester3"); err != nil {
			t.Error(err)
		}
		t.Log(ntr.Name)
		return errors.New("TransactionError")
	})

	ntr2 := &Tester{}
	if err := orm.First(ctx, ntr2, "name = ?", "Tester3"); err != nil {
		t.Error(err)
	}
	t.Log(ntr2.Name)

}
