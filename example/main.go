package main

//import (
//	"context"
//	"errors"
//	"fmt"
//	"github.com/gin-gonic/gin"
//	"github.com/goodbye-jack/go-common/config"
//	myHttp "github.com/goodbye-jack/go-common/http"
//	"github.com/goodbye-jack/go-common/log"
//	"github.com/goodbye-jack/go-common/model"
//	"github.com/goodbye-jack/go-common/orm"
//	"github.com/goodbye-jack/go-common/utils"
//)
//
//type World struct {
//	Name string `json:"name"`
//}
//
//func recordOp(ctx context.Context, op myHttp.Operation) error {
//	fmt.Println("%+v", op)
//	return nil
//}
//
//func init() {
//	log.Infof("打印数据是否开始加载")
//	myHttp.PreloadRouteAPI("/hello", "", []string{"GET"}, []string{utils.UserAnonymous}, false, false, func(c *gin.Context) {
//		world := World{
//			Name: "China",
//		}
//		myHttp.JsonResponse(c, world, nil)
//	})
//	myHttp.PreloadRouteAPI("/hello/error", "", []string{"GET"}, []string{utils.UserAnonymous}, false, false, func(c *gin.Context) {
//		world := World{
//			Name: "China",
//		}
//		myHttp.JsonResponse(c, world, errors.New("error"))
//	})
//	log.Info("数据加载结束")
//}
//
//func main() {
//	addr := config.GetConfigString("addr")
//	service_name := config.GetConfigString("service_name")
//	//log.LoadPrintProjectName("go-common")
//	//dsn := "host=113.45.4.22 port=4321 user=root password=Qaz0529! dbname=kingbase sslmode=disable TimeZone=Asia/Shanghai"
//	//orm.NewOrm(dsn, ormConfig.DBTypeKingBase, 3600)
//	//server := myHttp.NewHTTPServer(service_name)
//	myHttp.InitServer(service_name)
//	myHttp.GlobalServer.StaticFs("/static")
//	myHttp.GlobalServer.SetOpRecordFn(recordOp)
//	// 2. Mongo调用（无论单点/集群，调用方式一致）
//	coll := orm.Mongo.Collection("startup_log")
//	log.Info(coll)
//	_, errC := orm.Mongo.InsertOneWithCtx(context.Background(), "startup_log", map[string]interface{}{
//		"id":       "3454534543534534",
//		"hostname": "test",
//	})
//	if errC != nil {
//		return
//	}
//	list := []model.MenuBase{}
//	err := orm.DB.FindAll(context.Background(), &list)
//	if err != nil {
//		log.Error(err)
//	}
//	log.Error(list)
//	myHttp.GlobalServer.Prepare()
//	myHttp.GlobalServer.Run(addr)
//
//}
