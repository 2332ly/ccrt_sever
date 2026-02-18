package config

import (
	"ccrt_sever/global"
	"ccrt_sever/models"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func initDB() {
	dsn := AppConfig.Database.Dsn

	// 1) 确保数据库存在
	if err := ensureDatabaseExists(dsn); err != nil {
		log.Fatal(err)
	}

	// 2) 打开GORM连接
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal(err)
	}
	sqlDB.SetMaxIdleConns(AppConfig.Database.MaxIdleConns)
	sqlDB.SetMaxOpenConns(AppConfig.Database.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Hour)
	global.Db = db

	// 3) 自动迁移表结构
	if err := global.Db.AutoMigrate(
		&models.User{},
		&models.RefreshToken{},
		&models.Medication{},
		&models.MedicationCheckin{},
		&models.MedicationDispatch{},
		&models.Scale{},
		&models.ScaleVersion{},
		&models.ScaleModule{},
		&models.ScaleQuestion{},
		&models.ScaleAssessment{},
		&models.ScaleAnswer{},
		&models.AssessmentModuleScore{},
		&models.GameResult{},
		&models.SosEvent{},
	); err != nil {
		log.Fatal(err)
	}

	if err := ensureMMSESeed(); err != nil {
		log.Fatal(err)
	}
}

func ensureDatabaseExists(dsn string) error {
	baseDSN, dbName, err := splitMySQLDSN(dsn)
	if err != nil {
		return err
	}
	if dbName == "" {
		return errors.New("DSN 中未包含数据库名")
	}

	sdb, err := sql.Open("mysql", baseDSN)
	if err != nil {
		return err
	}
	defer func() { _ = sdb.Close() }()

	// 检查连接
	if err := sdb.Ping(); err != nil {
		return err
	}

	quoted := "`" + strings.ReplaceAll(dbName, "`", "``") + "`"
	if _, err := sdb.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s DEFAULT CHARACTER SET utf8mb4 DEFAULT COLLATE utf8mb4_general_ci", quoted)); err != nil {
		return err
	}
	return nil
}

// splitMySQLDSN 将 "user:pass@tcp(host:3306)/dbname?params" 拆分为：
// - baseDSN：不带库名的 DSN，例如 "user:pass@tcp(host:3306)/?params"（用于先创建数据库）
// - dbName：库名，例如 "dbname"
func splitMySQLDSN(dsn string) (baseDSN string, dbName string, err error) {
	// 匹配：...)/<dbname><可选的 ?...>
	// 示例：
	// root:pwd@tcp(127.0.0.1:3306)/ccrt?charset=utf8mb4&parseTime=True&loc=Local
	// user@tcp(localhost:3306)/test
	re := regexp.MustCompile(`\)/([^?]+)(\?.*)?$`)
	m := re.FindStringSubmatch(dsn)
	if len(m) < 2 {
		return "", "", fmt.Errorf("MySQL DSN 不合法（无法解析数据库名部分）：%s", dsn)
	}
	dbName = m[1]

	q := ""
	if len(m) >= 3 {
		q = m[2]
	}
	if q != "" {
		// 校验 query 字符串，避免拼出来的 DSN 不合法
		if _, perr := url.ParseQuery(strings.TrimPrefix(q, "?")); perr != nil {
			return "", "", fmt.Errorf("MySQL DSN 的 query 不合法：%w", perr)
		}
	}

	baseDSN = re.ReplaceAllString(dsn, ")/"+q)
	return baseDSN, dbName, nil
}
