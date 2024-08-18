package orm

import (
	"testing"
	"gorm.io/gorm"
)


type Tester struct {
	gorm.Model

	Name string
}

func TestORM(t *testing.T) {
	orm := NewOrm()
	orm.AutoMigrate(&Tester{})

	tr := Tester {
		Name: "Tester",
	}

	orm.Create(&tr)

	ntr := &Tester {}
	if err := orm.First(ntr, "name = ?", "Tester"); err !=  nil {
		t.Error(err)
	}

	t.Log(ntr.Name)

} 
