package wsqlx

import (
	"context"
	"database/sql"
	"github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

type Rdbms interface {
	// reader command

	QuerySq(ctx context.Context, query squirrel.Sqlizer, callback callbackRows) error
	QuerySqPagination(ctx context.Context, countQuery, query squirrel.SelectBuilder, pagination PaginationInput, callback callbackRows) (PaginationOutput, error)
	QueryRowSq(ctx context.Context, query squirrel.Sqlizer, scanType QueryRowScanType, dest interface{}) error

	// writer command

	ExecSq(ctx context.Context, query squirrel.Sqlizer) (sql.Result, error)
}

type queryExecutor interface {
	QueryxContext(ctx context.Context, query string, arg ...interface{}) (*sqlx.Rows, error)
	ExecContext(ctx context.Context, query string, arg ...interface{}) (sql.Result, error)
	QueryRowxContext(ctx context.Context, query string, args ...interface{}) *sqlx.Row
}
