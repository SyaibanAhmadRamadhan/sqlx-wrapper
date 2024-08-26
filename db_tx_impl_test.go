package wsqlx_test

import (
	"context"
	"database/sql"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	wsqlx "github.com/SyaibanAhmadRamadhan/sqlx-wrapper"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_sqlxTransaction_DoTransaction(t *testing.T) {
	dbMock, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer dbMock.Close()

	ctx := context.TODO()
	sqlxDB := sqlx.NewDb(dbMock, "sqlmock")

	sqlxx := wsqlx.NewSqlxTransaction(sqlxDB)

	t.Run("should be return with commit db", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectCommit()

		err = sqlxx.DoTx(ctx, &sql.TxOptions{}, func(tx wsqlx.Rdbms) (err error) {
			return nil
		})
		require.NoError(t, err)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("should be return with rollback db", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectRollback()

		err = sqlxx.DoTx(ctx, &sql.TxOptions{}, func(tx wsqlx.Rdbms) (err error) {
			return errors.New("error")
		})
		require.Error(t, err)

		require.NoError(t, mock.ExpectationsWereMet())
	})
}
