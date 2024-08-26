package wsqlx

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Masterminds/squirrel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func NewRdbms(db queryExecutor) *rdbms {
	tp := otel.GetTracerProvider()
	return &rdbms{
		db:     db,
		tracer: tp.Tracer(TracerName, trace.WithInstrumentationVersion(InstrumentVersion)),
	}
}

type rdbms struct {
	db     queryExecutor
	tracer trace.Tracer
}

func (s *rdbms) QuerySq(ctx context.Context, query squirrel.Sqlizer, callback callbackRows) error {
	rawQuery, args, err := query.ToSql()
	if err != nil {
		return errTracer(err)
	}

	ctx, spanQueryx := s.tracer.Start(ctx, rawQuery, []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(DBQueryTextKey.String(rawQuery)),
		trace.WithAttributes(makeParamAttr(args)),
		trace.WithAttributes(DBOperationNameKey.String(sqlOperationName(rawQuery))),
	}...)
	defer spanQueryx.End()

	res, err := s.db.QueryxContext(ctx, rawQuery, args...)
	if err != nil {
		recordError(spanQueryx, err)
		return err
	}
	defer func() {
		if errClose := res.Close(); errClose != nil {
			recordError(spanQueryx, errClose)
			spanQueryx.SetAttributes(attribute.String("db.system.close.rows", "failed"))
		} else {
			spanQueryx.SetAttributes(attribute.String("db.system.close.rows", "successfully"))
		}
	}()

	return callback(res)
}

func (s *rdbms) ExecSq(ctx context.Context, query squirrel.Sqlizer) (sql.Result, error) {
	rawQuery, args, err := query.ToSql()
	if err != nil {
		return nil, errTracer(err)
	}

	ctx, spanExec := s.tracer.Start(ctx, rawQuery, []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(DBQueryTextKey.String(rawQuery)),
		trace.WithAttributes(makeParamAttr(args)),
		trace.WithAttributes(DBOperationNameKey.String(sqlOperationName(rawQuery))),
	}...)
	defer spanExec.End()

	res, err := s.db.ExecContext(ctx, rawQuery, args...)
	if err != nil {
		recordError(spanExec, err)
		return nil, err
	}

	return res, nil
}

func (s *rdbms) QueryRowSq(ctx context.Context, query squirrel.Sqlizer, scanType QueryRowScanType, dest interface{}) error {
	rawQuery, args, err := query.ToSql()
	if err != nil {
		return errTracer(err)
	}

	ctx, spanQueryx := s.tracer.Start(ctx, rawQuery, []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(DBQueryTextKey.String(rawQuery)),
		trace.WithAttributes(makeParamAttr(args)),
		trace.WithAttributes(DBOperationNameKey.String(sqlOperationName(rawQuery))),
	}...)
	defer spanQueryx.End()

	res := s.db.QueryRowxContext(ctx, rawQuery, args...)

	switch scanType {
	case QueryRowScanTypeStruct:
		err = res.StructScan(dest)
	default:
		err = res.Scan(dest)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = errors.Join(err, ErrRecordNoRows)
		} else {
			recordError(spanQueryx, err)
		}

		return errTracer(err)
	}
	return nil
}

func (s *rdbms) QuerySqPagination(ctx context.Context, countQuery, query squirrel.SelectBuilder, paginationInput PaginationInput, callback callbackRows) (
	PaginationOutput, error) {

	offset := paginationInput.Offset()
	query = query.Limit(uint64(paginationInput.PageSize))
	query = query.Offset(uint64(offset))

	totalData := int64(0)
	err := s.QueryRowSq(ctx, countQuery, QueryRowScanTypeDefault, &totalData)
	if err != nil {
		return PaginationOutput{}, errTracer(err)
	}

	err = s.QuerySq(ctx, query, callback)
	if err != nil {
		return PaginationOutput{}, errTracer(err)
	}

	return CreatePaginationOutput(paginationInput, totalData), nil
}
