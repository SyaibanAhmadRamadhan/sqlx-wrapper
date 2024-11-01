package wsqlx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"runtime/debug"
	"strings"
)

// SpanNameFunc is a function that can be used to generate a span name for a
// SQL. The function will be called with the SQL statement as a parameter.
type SpanNameFunc func(stmt string) string
type optionFunc func(*rdbms)

func WithAttributes(attrs ...attribute.KeyValue) optionFunc {
	return func(cfg *rdbms) {
		cfg.attrs = append(cfg.attrs, attrs...)
	}
}

// WithSpanNameFunc will use the provided function to generate the span name for
// a SQL statement. The function will be called with the SQL statement as a
// parameter.
//
// By default, the whole SQL statement is used as a span name, where applicable.
func WithSpanNameFunc(fn SpanNameFunc) optionFunc {
	return func(cfg *rdbms) {
		cfg.spanNameFunc = fn
	}
}

func WithOutIncludeQueryParameters() optionFunc {
	return func(cfg *rdbms) {
		cfg.includeParams = false
	}
}

func WithConfig(port int, host, user string) optionFunc {
	return func(cfg *rdbms) {
		cfg.rdbmsConfig = &rdbmsConfig{
			host: host,
			port: port,
			user: user,
		}
	}
}

func findOwnImportedVersion() string {
	buildInfo, ok := debug.ReadBuildInfo()
	if ok {
		for _, dep := range buildInfo.Deps {
			if dep.Path == TracerName {
				return dep.Version
			}
		}
	}

	return "unknown"
}

func defaultSpanNameFN(s string) string {
	return fmt.Sprintf("%s...", s[0:15])
}

func NewRdbms(db *sqlx.DB, opt ...optionFunc) *rdbms {
	tp := otel.GetTracerProvider()
	r := &rdbms{
		db:             db,
		queryExecutor:  db,
		tracer:         tp.Tracer(TracerName, trace.WithInstrumentationVersion(findOwnImportedVersion())),
		tracerProvider: tp,
		attrs:          nil,
		spanNameFunc:   defaultSpanNameFN,
		includeParams:  true,
		rdbmsConfig:    nil,
	}

	for _, o := range opt {
		o(r)
	}

	return r
}

type rdbms struct {
	db            *sqlx.DB
	queryExecutor queryExecutor

	tracer         trace.Tracer
	tracerProvider trace.TracerProvider
	attrs          []attribute.KeyValue
	spanNameFunc   SpanNameFunc
	includeParams  bool
	rdbmsConfig    *rdbmsConfig
}

type rdbmsConfig struct {
	host string
	port int
	user string
}

// sqlOperationName attempts to get the first 'word' from a given SQL query, which usually
// is the operation name (e.g. 'SELECT').
func (s *rdbms) sqlOperationName(stmt string) string {
	if s.spanNameFunc != nil {
		return s.spanNameFunc(stmt)
	}

	parts := strings.Fields(stmt)
	if len(parts) == 0 {
		// Fall back to a fixed value to prevent creating lots of tracing operations
		// differing only by the amount of whitespace in them (in case we'd fall back
		// to the full query or a cut-off version).
		return sqlOperationUnknown
	}
	return strings.ToUpper(parts[0])
}

// commonAttribute returns a slice of SpanStartOptions that contain
// attributes from the given connection config and common attribute like query text or query param
func (s *rdbms) commonAttribute(rawQuery string, args ...interface{}) []trace.SpanStartOption {
	attrs := []trace.SpanStartOption{
		trace.WithAttributes(semconv.DBOperationName(s.sqlOperationName(rawQuery))),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(semconv.DBQueryText(rawQuery)),
	}
	if s.rdbmsConfig != nil {
		attrs = append(attrs, []trace.SpanStartOption{
			trace.WithAttributes(
				semconv.ServerAddress(s.rdbmsConfig.host),
				semconv.ServerPort(s.rdbmsConfig.port),
			),
		}...)
	}

	if s.includeParams {
		attrs = append(attrs, trace.WithAttributes(makeParamAttr(args)))
	}
	if s.attrs != nil {
		attrs = append(attrs, trace.WithAttributes(s.attrs...))
	}

	return attrs
}

func (s *rdbms) QuerySq(ctx context.Context, query squirrel.Sqlizer, callback callbackRows) error {
	rawQuery, args, err := query.ToSql()
	if err != nil {
		return errTracer(err)
	}

	ctx, spanQueryx := s.tracer.Start(ctx, s.spanNameFunc(rawQuery), s.commonAttribute(rawQuery, args)...)
	defer spanQueryx.End()

	res, err := s.queryExecutor.QueryxContext(ctx, rawQuery, args...)
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

	ctx, spanExec := s.tracer.Start(ctx, s.spanNameFunc(rawQuery), s.commonAttribute(rawQuery, args)...)
	defer spanExec.End()

	res, err := s.queryExecutor.ExecContext(ctx, rawQuery, args...)
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

	ctx, spanQueryx := s.tracer.Start(ctx, s.spanNameFunc(rawQuery), s.commonAttribute(rawQuery, args)...)
	defer spanQueryx.End()

	res := s.queryExecutor.QueryRowxContext(ctx, rawQuery, args...)

	switch scanType {
	case QueryRowScanTypeStruct:
		err = res.StructScan(dest)
	default:
		err = res.Scan(dest)
	}
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
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

func (s *rdbms) injectTx(tx *sqlx.Tx) *rdbms {
	newRdbms := *s
	newRdbms.queryExecutor = tx
	return &newRdbms
}

func (s *rdbms) DoTx(ctx context.Context, opt *sql.TxOptions, fn func(tx Rdbms) (err error)) (err error) {
	opts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(DBTxIsolationLevel.String(opt.Isolation.String())),
		trace.WithAttributes(DBTxReadOnly.Bool(opt.ReadOnly)),
	}

	spanName := "do transaction"

	_, span := s.tracer.Start(ctx, spanName, opts...)
	defer span.End()

	tx, err := s.db.BeginTxx(ctx, opt)
	if err != nil {
		recordError(span, err)
		return errTracer(err)
	}

	defer func() {
		if p := recover(); p != nil {
			span.SetAttributes(attribute.String("db.tx.operation", "rollback"))
			errRollback := tx.Rollback()
			if errRollback != nil {
				recordError(span, errRollback)
				span.SetAttributes(attribute.String("db.tx.status", "rollback failed"))
			} else {
				span.SetAttributes(attribute.String("db.tx.status", "rollback successfully"))
			}
			recordError(span, fmt.Errorf("panic occurred: %v", p))
			panic(p)
		} else if err != nil {
			span.SetAttributes(attribute.String("db.tx.operation", "rollback"))
			if errRollback := tx.Rollback(); errRollback != nil {
				recordError(span, errRollback)
				err = errors.Join(err, errRollback)
				span.SetAttributes(attribute.String("db.tx.status", "rollback failed"))
			} else {
				span.SetAttributes(attribute.String("db.tx.status", "rollback successfully"))
			}
		} else {
			span.SetAttributes(attribute.String("db.tx.operation", "commit"))
			if errCommit := tx.Commit(); errCommit != nil {
				recordError(span, errCommit)
				err = errCommit
				span.SetAttributes(attribute.String("db.tx.status", "commit failed"))
			} else {
				span.SetAttributes(attribute.String("db.tx.status", "commit successfully"))
			}
		}
	}()

	err = fn(s.injectTx(tx))
	if err != nil {
		recordError(span, err)
	}
	return
}

func (s *rdbms) DoTxContext(ctx context.Context, opt *sql.TxOptions, fn func(ctx context.Context, tx Rdbms) (err error)) (err error) {
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
			span.SetAttributes(attribute.String("db.tx.operation", "rollback"))
			errRollback := tx.Rollback()
			if errRollback != nil {
				recordError(span, errRollback)
				span.SetAttributes(attribute.String("db.tx.status", "rollback failed"))
			} else {
				span.SetAttributes(attribute.String("db.tx.status", "rollback successfully"))
			}
			recordError(span, fmt.Errorf("panic occurred: %v", p))
			panic(p)
		} else if err != nil {
			span.SetAttributes(attribute.String("db.tx.operation", "rollback"))
			if errRollback := tx.Rollback(); errRollback != nil {
				recordError(span, errRollback)
				err = errors.Join(err, errRollback)
				span.SetAttributes(attribute.String("db.tx.status", "rollback failed"))
			} else {
				span.SetAttributes(attribute.String("db.tx.status", "rollback successfully"))
			}
		} else {
			span.SetAttributes(attribute.String("db.tx.operation", "commit"))
			if errCommit := tx.Commit(); errCommit != nil {
				recordError(span, errCommit)
				err = errCommit
				span.SetAttributes(attribute.String("db.tx.status", "commit failed"))
			} else {
				span.SetAttributes(attribute.String("db.tx.status", "commit successfully"))
			}
		}
	}()

	err = fn(ctx, s.injectTx(tx))
	return
}
