package bootstrap

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	_ "github.com/go-sql-driver/mysql"
)

func OpenDB(cfg config.DBConfig) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql connection: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping mysql connection: %w", err)
	}

	return db, nil
}

type DBSet struct {
	Core    *sql.DB
	Runtime *sql.DB
	Derived *sql.DB
}

func OpenDBSet(cfg *config.Config) (*DBSet, error) {
	coreDB, err := OpenDB(cfg.CoreDB)
	if err != nil {
		return nil, fmt.Errorf("open core db: %w", err)
	}
	runtimeDB, err := OpenDB(cfg.RuntimeDB)
	if err != nil {
		coreDB.Close()
		return nil, fmt.Errorf("open runtime db: %w", err)
	}
	derivedDB, err := OpenDB(cfg.DerivedDB)
	if err != nil {
		coreDB.Close()
		runtimeDB.Close()
		return nil, fmt.Errorf("open derived db: %w", err)
	}
	return &DBSet{
		Core:    coreDB,
		Runtime: runtimeDB,
		Derived: derivedDB,
	}, nil
}

func (s *DBSet) Close() {
	if s == nil {
		return
	}
	if s.Core != nil {
		_ = s.Core.Close()
	}
	if s.Runtime != nil {
		_ = s.Runtime.Close()
	}
	if s.Derived != nil {
		_ = s.Derived.Close()
	}
}
