package model

type MenuBase struct {
	ModelBase
	ParentId    uint   `json:"parent_id"`
	MenuName    string `json:"menu_name"`
	MenuType    uint8  `json:"menu_type"`
	MenuTitle   string `json:"menu_title"`
	ServiceName string `json:"service_name"`
	Path        string `json:"path"   gorm:"uniqueIndex:uniq_path_method;type:varchar(128)"`
	Method      string `json:"method" gorm:"uniqueIndex:uniq_path_method;type:varchar(64)"`
	Component   string `json:"component"`
	Icon        string `json:"icon"`
	IconType    string `json:"icon_type"`
	Visible     int    `json:"visible"`
	Sort        int    `json:"sort"`
	RoleCode    string `json:"role_code"` // 是一个以逗号分隔的roleCode编码
	Status      int    `json:"status"`
}
