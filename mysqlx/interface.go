package mysqlx

import "gorm.io/gorm"

type Model interface {
	ID() uint64
	GetCreateDDL() string
	TableName() string
}

type Session interface {
	DB() *gorm.DB
	CreateTableIfNotExists(table Model) error
	IsInTransaction() bool
}
