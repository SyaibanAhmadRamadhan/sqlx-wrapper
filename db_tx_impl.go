package wsqlx

import (
	"context"
	"database/sql"
	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func NewSqlxTransaction(db *sqlx.DB) *sqlxTransaction {
	tp := otel.GetTracerProvider()
	return &sqlxTransaction{
		db:     db,
		tracer: tp.Tracer(TracerName, trace.WithInstrumentationVersion(InstrumentVersion)),
	}
}

type sqlxTransaction struct {
	db     *sqlx.DB
	tracer trace.Tracer
}

func (s *sqlxTransaction) DoTx(ctx context.Context, opt *sql.TxOptions, fn func(tx Rdbms) (err error)) (err error) {
	opts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(DBTxIsolationLevel.String(opt.Isolation.String())),
		trace.WithAttributes(DBTxReadOnly.Bool(opt.ReadOnly)),
	}

	spanName := "do transaction"

	ctx, span := s.tracer.Start(ctx, spanName, opts...)
	defer span.End()

	tx, err := s.db.BeginTxx(ctx, opt)
	if err != nil {
		recordError(span, err)
		return errTracer(err)
	}

	defer func() {
		if p := recover(); p != nil {
			err = tx.Rollback()
			recordError(span, err)
			panic(p)
		} else if err != nil {
			if errRollback := tx.Rollback(); errRollback != nil {
				recordError(span, errRollback)
				err = errRollback
			}
		} else {
			if errCommit := tx.Commit(); errCommit != nil {
				recordError(span, errCommit)
				err = errCommit
			}
		}
	}()

	sqlxWrapper := NewRdbms(tx)

	err = fn(sqlxWrapper)
	return
}
