package mysqlx

import (
	"errors"
	"fmt"
	"sync"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/jiajia556/tool-box/syncmap"
)

type MysqlConfig struct {
	Host            string          `json:"host" yaml:"host"`
	User            string          `json:"user" yaml:"user"`
	Password        string          `json:"password" yaml:"password"`
	DBName          string          `json:"db_name" yaml:"db_name"`
	Port            int             `json:"port" yaml:"port"`
	Prefix          string          `json:"prefix" yaml:"prefix"`
	Charset         string          `json:"charset" yaml:"charset"`
	LogLevel        logger.LogLevel `json:"log_level" yaml:"log_level"`
	AutoCreateTable bool            `json:"auto_create_table" yaml:"auto_create_table"`

	// Optional connection pool settings; zero means "use driver default".
	MaxOpenConns    int           `json:"max_open_conns" yaml:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns" yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" yaml:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time" yaml:"conn_max_idle_time"`
}

type tableState struct {
	mu   sync.Mutex
	done bool
}

// SqlDB .
var (
	sqlDB           *gorm.DB
	autoCreateTable bool
	tableStates     syncmap.SyncMap[string, *tableState]
)

// InitMysql .
func InitMysql(conf MysqlConfig) error {
	dsnCfg := &mysqlDriver.Config{
		User:      conf.User,
		Passwd:    conf.Password,
		Net:       "tcp",
		Addr:      fmt.Sprintf("%s:%d", conf.Host, conf.Port),
		DBName:    conf.DBName,
		Loc:       time.Local,
		ParseTime: true,
		Params: map[string]string{
			"charset":              conf.Charset,
			"allowNativePasswords": "true",
		},
	}

	var err error
	sqlDB, err = gorm.Open(
		mysql.Open(dsnCfg.FormatDSN()), &gorm.Config{
			NamingStrategy: schema.NamingStrategy{
				TablePrefix:   conf.Prefix, // table name prefix
				SingularTable: true,        // use singular table name
			},
			Logger: logger.Default.LogMode(conf.LogLevel),
		})
	if err != nil {
		return err
	}

	// Configure connection pool if provided.
	if rawDB, err := sqlDB.DB(); err == nil {
		if conf.MaxOpenConns > 0 {
			rawDB.SetMaxOpenConns(conf.MaxOpenConns)
		}
		if conf.MaxIdleConns > 0 {
			rawDB.SetMaxIdleConns(conf.MaxIdleConns)
		}
		if conf.ConnMaxLifetime > 0 {
			rawDB.SetConnMaxLifetime(conf.ConnMaxLifetime)
		}
		if conf.ConnMaxIdleTime > 0 {
			rawDB.SetConnMaxIdleTime(conf.ConnMaxIdleTime)
		}
	}

	autoCreateTable = conf.AutoCreateTable
	return nil
}

// GetDB returns the global *gorm.DB instance; it panics if MySQL hasn't been initialized.
func GetDB() *gorm.DB {
	if sqlDB == nil {
		panic("MySQL not initialized")
	}
	return sqlDB
}

func AutoCreateTable() bool {
	return autoCreateTable
}

func NewTxSession() *TxSession {
	return &TxSession{base: GetDB()}
}

type TxSession struct {
	base *gorm.DB
	tx   *gorm.DB
}

// Begin starts a transaction; returns an error if a transaction is already active.
func (m *TxSession) Begin() error {
	if m == nil {
		return errors.New("nil TxSession")
	}
	if m.base == nil {
		m.base = GetDB()
	}
	if m.tx != nil {
		return errors.New("transaction already started")
	}
	tx := m.base.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	m.tx = tx
	return nil
}

// Commit commits the transaction; it can only be called when a transaction is active.
func (m *TxSession) Commit() error {
	if m == nil {
		return errors.New("nil TxSession")
	}
	if m.tx == nil {
		return errors.New("no active transaction")
	}
	err := m.tx.Commit().Error
	m.tx = nil
	return err
}

// Rollback rolls back the transaction; it can only be called when a transaction is active.
func (m *TxSession) Rollback() error {
	if m == nil {
		return errors.New("nil TxSession")
	}
	if m.tx == nil {
		return errors.New("no active transaction")
	}
	err := m.tx.Rollback().Error
	m.tx = nil
	return err
}

// InTx runs fn within a transaction scope.
// - fn returns error: rollback and return that error (wrap if rollback fails)
// - fn panics: rollback and re-panic (don't swallow the failure)
// - fn returns nil: commit
func (m *TxSession) InTx(fn func(tx *gorm.DB) error) (err error) {
	if m == nil {
		return errors.New("nil TxSession")
	}
	if fn == nil {
		return errors.New("nil tx function")
	}
	if m.tx != nil {
		return errors.New("already in transaction")
	}
	if err = m.Begin(); err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			_ = m.Rollback()
			panic(r)
		}
		if err != nil {
			if rbErr := m.Rollback(); rbErr != nil {
				err = fmt.Errorf("%w; rollback error: %v", err, rbErr)
			}
			return
		}
		err = m.Commit()
	}()

	return fn(m.tx)
}

func (m *TxSession) IsInTransaction() bool {
	if m == nil {
		return false
	}
	return m.tx != nil
}

// DB returns the *gorm.DB that should be used currently:
// - if a transaction is active: return the transactional handle
// - otherwise: return the base connection
func (m *TxSession) DB() *gorm.DB {
	if m == nil {
		panic("nil TxSession")
	}
	if m.tx != nil {
		return m.tx
	}
	if m.base == nil {
		m.base = GetDB()
	}
	return m.base
}

func (m *TxSession) CreateTableIfNotExists(table Model) error {
	db := m.DB()

	tableName := table.TableName()

	state, _ := tableStates.LoadOrStore(tableName, &tableState{})

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.done {
		return nil
	}

	migrator := db.Migrator()

	if !migrator.HasTable(table) {
		if err := db.Exec(table.GetCreateDDL()).Error; err != nil {
			return err
		}
	}

	state.done = true
	return nil
}
