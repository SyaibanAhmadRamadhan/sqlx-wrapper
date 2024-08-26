package wsqlx

import (
	"context"
	"database/sql"
)

type Tx interface {
	DoTx(ctx context.Context, opt *sql.TxOptions, fn func(tx Rdbms) error) error
}
