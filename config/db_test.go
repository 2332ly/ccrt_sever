package config

import "testing"

func TestSplitMySQLDSN_WithQuery(t *testing.T) {
	base, db, err := splitMySQLDSN("root:pwd@tcp(127.0.0.1:3306)/ccrt?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		t.Fatalf("不应该返回错误：%v", err)
	}
	if db != "ccrt" {
		t.Fatalf("数据库名不匹配：得到 %q", db)
	}
	wantBase := "root:pwd@tcp(127.0.0.1:3306)/?charset=utf8mb4&parseTime=True&loc=Local"
	if base != wantBase {
		t.Fatalf("基础 DSN 不匹配：\n得到：%q\n期望：%q", base, wantBase)
	}
}

func TestSplitMySQLDSN_NoQuery(t *testing.T) {
	base, db, err := splitMySQLDSN("user:pass@tcp(localhost:3306)/test")
	if err != nil {
		t.Fatalf("不应该返回错误：%v", err)
	}
	if db != "test" {
		t.Fatalf("数据库名不匹配：得到 %q", db)
	}
	wantBase := "user:pass@tcp(localhost:3306)/"
	if base != wantBase {
		t.Fatalf("基础 DSN 不匹配：得到 %q，期望 %q", base, wantBase)
	}
}

func TestSplitMySQLDSN_Invalid(t *testing.T) {
	if _, _, err := splitMySQLDSN("not-a-dsn"); err == nil {
		t.Fatalf("期望返回错误，但没有返回")
	}
}
