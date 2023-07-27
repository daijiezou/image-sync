package mysql

import (
	"xorm.io/xorm"
	"xorm.io/xorm/log"
	_ "github.com/go-sql-driver/mysql"
)
var mysqlClient *xorm.Engine

func InitMySQL(dbDsn string) error {
	var err error
	mysqlClient, err = xorm.NewEngine("mysql", dbDsn)
	if err != nil {
		return err
	}
	err = mysqlClient.Ping()
	if err != nil {
		return err
	}
	mysqlClient.ShowSQL(true)
	mysqlClient.SetMaxIdleConns(20)

	mysqlClient.Logger().SetLevel(log.LOG_WARNING)
	return nil
}

func MySQL() *xorm.Engine {
	return mysqlClient
}
